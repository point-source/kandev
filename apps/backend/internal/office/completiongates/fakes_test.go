package completiongates

import (
	"context"
	"slices"
	"sync"
	"time"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeGates struct {
	mu        sync.Mutex
	snapshots map[string]*models.TaskCompletionGateSnapshot
	recorded  []models.TaskCompletionEvaluationChange
	controls  []models.TaskCompletionControlChange
	unmet     []string
	queries   []models.TaskCompletionSweepQuery
	// heldUnescalated answers HeldUnescalated sweep queries.
	heldUnescalated []string
	changed         bool
}

func newFakeGates() *fakeGates {
	return &fakeGates{snapshots: map[string]*models.TaskCompletionGateSnapshot{}, changed: true}
}

func (f *fakeGates) setCriteria(taskID string, criteria ...models.TaskCompletionCriterion) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.snapshots[taskID] = &models.TaskCompletionGateSnapshot{TaskID: taskID, Revision: 1, Criteria: criteria}
}

func (f *fakeGates) GetTaskCompletionGate(_ context.Context, taskID string) (*models.TaskCompletionGateSnapshot, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if snapshot, ok := f.snapshots[taskID]; ok {
		copied := *snapshot
		copied.Criteria = append([]models.TaskCompletionCriterion(nil), snapshot.Criteria...)
		return &copied, nil
	}
	return &models.TaskCompletionGateSnapshot{TaskID: taskID}, nil
}

func (f *fakeGates) RecordTaskCompletionEvaluation(_ context.Context, change models.TaskCompletionEvaluationChange) (*models.TaskCompletionEvaluationOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.recorded = append(f.recorded, change)
	criterion := models.TaskCompletionCriterion{ID: change.CriterionID, Result: change.Result, HeldReason: change.Hold,
		ResultSubjectRevision: change.SubjectRevision, CriterionRevision: change.CriterionRevision}
	if snapshot := f.snapshots[change.TaskID]; snapshot != nil {
		for index := range snapshot.Criteria {
			if snapshot.Criteria[index].ID == change.CriterionID {
				stored := &snapshot.Criteria[index]
				stored.Result, stored.ResultSubjectRevision = change.Result, change.SubjectRevision
				criterion.Kind = stored.Kind
			}
		}
	}
	return &models.TaskCompletionEvaluationOutcome{Snapshot: &models.TaskCompletionGateSnapshot{TaskID: change.TaskID}, Criterion: criterion, Changed: f.changed}, nil
}

func (f *fakeGates) ApplyTaskCompletionControl(_ context.Context, change models.TaskCompletionControlChange) (*models.TaskCompletionControlOutcome, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.controls = append(f.controls, change)
	snapshot := f.snapshots[change.TaskID]
	claimed := false
	if snapshot == nil {
		snapshot = &models.TaskCompletionGateSnapshot{TaskID: change.TaskID}
		f.snapshots[change.TaskID] = snapshot
	}
	for index := range snapshot.Criteria {
		criterion := &snapshot.Criteria[index]
		if criterion.ID != change.CriterionID {
			continue
		}
		switch change.Action {
		case models.TaskCompletionControlCountRework:
			if !slices.Contains(criterion.ReworkKeys, change.Key) {
				if change.ReworkCap > 0 && criterion.ReworkCount >= change.ReworkCap {
					criterion.HeldReason = HoldReworkCapReached
				} else {
					criterion.ReworkKeys = append(criterion.ReworkKeys, change.Key)
					criterion.ReworkCount++
					claimed = true
				}
			}
		case models.TaskCompletionControlReleaseHold, models.TaskCompletionControlSendBackProtected:
			criterion.HeldReason = ""
		case models.TaskCompletionControlMarkEscalated:
			now := time.Now()
			criterion.EscalatedAt = &now
		}
	}
	copied := *snapshot
	copied.Criteria = append([]models.TaskCompletionCriterion(nil), snapshot.Criteria...)
	return &models.TaskCompletionControlOutcome{Snapshot: &copied, Claimed: claimed}, nil
}

func (f *fakeGates) ListTaskIDsWithUnmetCompletionCriteria(_ context.Context, query models.TaskCompletionSweepQuery) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queries = append(f.queries, query)
	source := f.unmet
	if query.HeldUnescalated {
		source = f.heldUnescalated
	}
	var page []string
	for _, id := range source {
		if id > query.AfterTaskID && len(page) < query.Limit {
			page = append(page, id)
		}
	}
	return page, nil
}

func (f *fakeGates) AuthorizeTaskScope(context.Context, string, authz.Scope) error { return nil }

func (f *fakeGates) AuthorizeWorkspaceScope(context.Context, string, authz.Scope) error { return nil }

func (f *fakeGates) records() []models.TaskCompletionEvaluationChange {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]models.TaskCompletionEvaluationChange(nil), f.recorded...)
}

type fakePRs struct {
	mu         sync.Mutex
	links      map[string][]PRLink
	checks     map[string]CheckObservation
	files      map[string]FilesObservation
	fileReads  int
	checkReads int
	filesErr   error
}

func newFakePRs() *fakePRs {
	return &fakePRs{links: map[string][]PRLink{}, checks: map[string]CheckObservation{}, files: map[string]FilesObservation{}}
}

func (f *fakePRs) LinkedPRs(_ context.Context, taskID string) ([]PRLink, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]PRLink(nil), f.links[taskID]...), nil
}

func (f *fakePRs) ObserveChecks(_ context.Context, _ string, link PRLink) (CheckObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.checkReads++
	return f.checks[link.ID], nil
}

func (f *fakePRs) ChangedFiles(_ context.Context, _ string, link PRLink) (FilesObservation, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.fileReads++
	if f.filesErr != nil {
		return FilesObservation{}, f.filesErr
	}
	return f.files[link.ID], nil
}
