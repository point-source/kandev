package completiongates

import (
	"context"
	"errors"
	"sync"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// VerifierKindApproval records whether an approval asks a person.
const VerifierKindApproval = "approval"

// ApprovalWaitsForWorkStage is the detail of an approval not yet asked
// because the task has not left the work stage.
const ApprovalWaitsForWorkStage = "work_stage"

// ApprovalWaitsForPullRequest is the detail of an approval not yet asked
// because the task has no linked pull request.
const ApprovalWaitsForPullRequest = "pull_request"

// taskLocks serializes approval evaluation and the approve control per task,
// so an evaluation never records an ask from a placement or gate state that a
// later evaluation already replaced.
type taskLocks struct {
	mu    sync.Mutex
	locks map[string]*taskLock
}

type taskLock struct {
	sync.Mutex
	users int
}

func (l *taskLocks) lock(taskID string) func() {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = map[string]*taskLock{}
	}
	entry := l.locks[taskID]
	if entry == nil {
		entry = &taskLock{}
		l.locks[taskID] = entry
	}
	entry.users++
	l.mu.Unlock()
	entry.Lock()
	return func() {
		entry.Unlock()
		l.mu.Lock()
		if entry.users--; entry.users == 0 {
			delete(l.locks, taskID)
		}
		l.mu.Unlock()
	}
}

// EvaluateApproval records whether the task's approval criterion asks a
// person at the live heads: once a pull request is linked, the task has left
// the work stage and every other criterion passes there. Until then it
// records what the approval waits for. A person's verdict at the live heads,
// and a held approval, stand.
func (r *Router) EvaluateApproval(ctx context.Context, taskID string) {
	unlock := r.approvals.lock(taskID)
	defer unlock()
	r.evaluateApprovalLocked(ctx, taskID)
}

// evaluateApprovalLocked is EvaluateApproval for a caller holding the task's
// approval lock. Without a linked pull request the approval waits for one,
// which also withdraws an ask after the last pull request is detached.
func (r *Router) evaluateApprovalLocked(ctx context.Context, taskID string) {
	snapshot, err := r.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return
	}
	approval, ok := findCriterionOfKind(snapshot, models.TaskCompletionCriterionApproval)
	if !ok || approval.HoldKind != "" {
		return
	}
	links, err := r.prs.LinkedPRs(ctx, taskID)
	if err != nil {
		return
	}
	waiting := []string{ApprovalWaitsForPullRequest}
	if len(links) > 0 {
		placement, ok := r.placement(ctx, taskID)
		if !ok {
			return
		}
		waiting = approvalWaitsFor(snapshot, approval.ID, placement)
	}
	change := models.TaskCompletionEvaluationChange{
		TaskID: taskID, CriterionID: approval.ID, CriterionRevision: approval.CriterionRevision,
		SubjectRevision: HeadsRevision(links), Result: models.TaskCompletionResultPending,
		Reason: models.TaskCompletionReasonAwaitingPerson, Detail: waiting,
		VerifierKind: VerifierKindApproval, VerifierID: VerifierKindApproval,
	}
	if len(waiting) > 0 {
		change.Reason = models.TaskCompletionReasonAwaitingCriteria
	}
	_, err = r.gates.RecordTaskCompletionEvaluation(ctx, change)
	if err != nil && !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		r.logger.Warn("completion gate approval evaluation failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

// approvalWaitsFor names what keeps an approval from asking a person: the
// work stage while the task has not left it, then the kind of every other
// criterion that does not pass at the live heads.
func approvalWaitsFor(snapshot *models.TaskCompletionGateSnapshot, approvalID string, placement Placement) []string {
	if placement.Terminal || (placement.WorkStepID != "" && !placement.pastWork()) {
		return []string{ApprovalWaitsForWorkStage}
	}
	blocked := map[string]bool{}
	for _, blocker := range snapshot.Blockers {
		blocked[blocker.CriterionID] = true
	}
	var kinds []string
	for _, criterion := range snapshot.Criteria {
		if criterion.ID != approvalID && blocked[criterion.ID] {
			kinds = append(kinds, models.NormalizeTaskCompletionCriterionKind(criterion.Kind))
		}
	}
	return kinds
}
