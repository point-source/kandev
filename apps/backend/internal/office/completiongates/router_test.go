package completiongates

import (
	"context"
	"sync"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

type fakeNavigator struct {
	mu        sync.Mutex
	placement Placement
	moves     []string
	retries   []string
}

func (f *fakeNavigator) TaskPlacement(context.Context, string) (Placement, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.placement, nil
}

func (f *fakeNavigator) MoveTask(_ context.Context, _, _, stepID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.moves = append(f.moves, stepID)
	return nil
}

func (f *fakeNavigator) RetryCompletingTransition(_ context.Context, _, _, stepID, operationID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.retries = append(f.retries, stepID+"|"+operationID)
	return nil
}

type spyEscalator struct{ held []string }

func (s *spyEscalator) EscalateHold(_ context.Context, _ string, criterion models.TaskCompletionCriterion) {
	s.held = append(s.held, criterion.HoldKind)
}

func officePlacement(current string) Placement {
	positions := map[string]int{"work": 1, "review": 2, "approval": 3}
	return Placement{WorkflowID: "wf", CurrentStepID: current, CurrentPosition: positions[current],
		WorkStepID: "work", WorkPosition: 1, ReviewStepID: "review", ReviewPosition: 2}
}

func setupRouter(t *testing.T, current string, criteria ...models.TaskCompletionCriterion) (*Router, *fakeGates, *fakeNavigator, *spyEscalator) {
	t.Helper()
	gates, prs := newFakeGates(), newFakePRs()
	gates.setCriteria("task-1", criteria...)
	prs.links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h2"}}
	navigator := &fakeNavigator{placement: officePlacement(current)}
	escalator := &spyEscalator{}
	review := NewReviewVerifier(gates, prs, fakeTasks{assignee: "worker"}, &fakeReviews{}, newTestLogger(t))
	router := NewRouter(gates, prs, navigator, review, newTestLogger(t))
	router.SetEscalator(escalator)
	return router, gates, navigator, escalator
}

func failedCI(revision string) models.TaskCompletionCriterion {
	criterion := ciCriterion()
	criterion.Result, criterion.ResultSubjectRevision = models.TaskCompletionResultFail, revision
	return criterion
}

// @covers AC-OFFICE-COMPLETION-GATES-006.1
func TestRouterReturnsFailedCIToTheWorkStage(t *testing.T) {
	router, gates, navigator, _ := setupRouter(t, "approval", ciCriterion())
	outcome := &models.TaskCompletionEvaluationOutcome{Criterion: failedCI("pr-1@h2"), Changed: true}
	router.AfterRecord(context.Background(), "task-1", outcome)
	router.AfterRecord(context.Background(), "task-1", outcome)
	if len(navigator.moves) != 1 || navigator.moves[0] != "work" {
		t.Fatalf("moves = %v", navigator.moves)
	}
	if got := gates.snapshots["task-1"].Criteria[0].ReworkCount; got != 1 {
		t.Fatalf("rework count = %d", got)
	}
}

func TestRouterLeavesAWorkingTaskInPlace(t *testing.T) {
	router, _, navigator, _ := setupRouter(t, "work", ciCriterion())
	router.AfterRecord(context.Background(), "task-1", &models.TaskCompletionEvaluationOutcome{Criterion: failedCI("pr-1@h2"), Changed: true})
	if len(navigator.moves) != 0 {
		t.Fatalf("moves = %v", navigator.moves)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-007.3
func TestRouterHoldsAtTheReworkCap(t *testing.T) {
	limited := ciCriterion()
	limited.ReworkCount = DefaultLimit
	router, gates, navigator, escalator := setupRouter(t, "approval", limited)
	router.AfterRecord(context.Background(), "task-1", &models.TaskCompletionEvaluationOutcome{Criterion: limited, Changed: true})
	failed := limited
	failed.Result, failed.ResultSubjectRevision = models.TaskCompletionResultFail, "pr-1@h2"
	router.AfterRecord(context.Background(), "task-1", &models.TaskCompletionEvaluationOutcome{Criterion: failed, Changed: true})
	if len(navigator.moves) != 0 || gates.snapshots["task-1"].Criteria[0].HoldKind != HoldReworkLimitReached {
		t.Fatalf("moves=%v criterion=%+v", navigator.moves, gates.snapshots["task-1"].Criteria[0])
	}
	if len(escalator.held) != 1 {
		t.Fatalf("escalations = %v", escalator.held)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-006.2, AC-OFFICE-COMPLETION-GATES-007.3
func TestRouterCountsAttemptsAndHoldsAtTheAttemptCap(t *testing.T) {
	router, _, _, _ := setupRouter(t, "approval", ciCriterion())
	change := models.TaskCompletionEvaluationChange{Result: models.TaskCompletionResultCannotEvaluate}
	router.Prepare(context.Background(), "task-1", ciCriterion(), &change)
	if !change.CountAttempt || change.Hold != "" {
		t.Fatalf("first attempt = %+v", change)
	}
	exhausted := ciCriterion()
	exhausted.AttemptCount = DefaultLimit
	change = models.TaskCompletionEvaluationChange{Result: models.TaskCompletionResultCannotEvaluate}
	router.Prepare(context.Background(), "task-1", exhausted, &change)
	if change.CountAttempt || change.Hold != HoldVerificationLimitReached {
		t.Fatalf("exhausted attempt = %+v", change)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-006.3
func TestRouterRetriesCompletionWhenEverythingPasses(t *testing.T) {
	router, _, navigator, _ := setupRouter(t, "approval", ciCriterion())
	passed := ciCriterion()
	passed.Result, passed.ResultSubjectRevision = models.TaskCompletionResultPass, "pr-1@h2"
	router.AfterRecord(context.Background(), "task-1", &models.TaskCompletionEvaluationOutcome{
		Criterion: passed, Snapshot: &models.TaskCompletionGateSnapshot{}, Changed: true,
	})
	if len(navigator.retries) != 1 || navigator.retries[0] != "approval|completion-gate:task-1:approval:pr-1@h2" {
		t.Fatalf("retries = %v", navigator.retries)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-004.5
func TestRouterCountsAReviewRejectionWithoutMoving(t *testing.T) {
	router, gates, navigator, _ := setupRouter(t, "work", reviewCriterion())
	router.review.reviews.(*fakeReviews).decisions = []*workflowmodels.WorkflowStepDecision{rejection("seat-a", "d-1", 1)}
	router.OnTaskMoved(context.Background(), "task-1", "review", "42")
	router.OnTaskMoved(context.Background(), "task-1", "review", "42")
	if got := gates.snapshots["task-1"].Criteria[0].ReworkCount; got != 1 || len(navigator.moves) != 0 {
		t.Fatalf("rework=%d moves=%v", got, navigator.moves)
	}
}

func TestRouterReturnsAnUnreviewedTaskToReview(t *testing.T) {
	router, gates, navigator, _ := setupRouter(t, "approval", reviewCriterion())
	gates.snapshots["task-1"].Blockers = []models.TaskCompletionBlocker{{CriterionID: "review", Reason: "unverified"}}
	router.OnTaskMoved(context.Background(), "task-1", "review", "43")
	if len(navigator.moves) != 1 || navigator.moves[0] != "review" {
		t.Fatalf("moves = %v", navigator.moves)
	}
}

func TestRouterBouncesFailingCIOutOfReview(t *testing.T) {
	router, _, navigator, _ := setupRouter(t, "review", failedCI("pr-1@h2"))
	router.OnTaskMoved(context.Background(), "task-1", "work", "44")
	if len(navigator.moves) != 1 || navigator.moves[0] != "work" {
		t.Fatalf("moves = %v", navigator.moves)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-004.3
func TestRouterMarksAPassedReviewStaleWhenHeadsMove(t *testing.T) {
	passed := reviewCriterion()
	passed.Result, passed.ResultSubjectRevision = models.TaskCompletionResultPass, "pr-1@h1"
	router, gates, navigator, _ := setupRouter(t, "approval", passed)
	gates.snapshots["task-1"].Criteria[0].Kind = models.TaskCompletionCriterionReview
	router.OnHeadsChanged(context.Background(), "task-1")
	records := gates.records()
	if len(records) != 1 || records[0].Result != models.TaskCompletionResultPending || records[0].SubjectRevision != "pr-1@h2" {
		t.Fatalf("records = %+v", records)
	}
	if len(navigator.moves) != 1 || navigator.moves[0] != "review" {
		t.Fatalf("moves = %v", navigator.moves)
	}
}
