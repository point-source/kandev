package completiongates

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

func rejection(participant, id string, minute int) *workflowmodels.WorkflowStepDecision {
	return &workflowmodels.WorkflowStepDecision{ID: id, ParticipantID: participant, Decision: decisionRejected,
		DeciderType: "agent", DeciderID: participant, DecidedAt: time.Date(2026, 11, 21, 10, minute, 0, 0, time.UTC)}
}

func TestChangesRequestedFailsReviewOverAHumanApproval(t *testing.T) {
	verifier, gates, reviews := setupReview(t)
	changes := rejection("seat-a", "d-changes", 2)
	changes.Decision = decisionChangesRequested
	reviews.decisions = []*workflowmodels.WorkflowStepDecision{approval("human-seat", "user", "default", "pr-1@h1"), changes}
	verifier.DecisionRecorded(context.Background(), "task-1", "review", "approved")
	records := gates.records()
	if len(records) != 1 || records[0].Result != models.TaskCompletionResultFail || records[0].EvidenceReference != "d-changes" {
		t.Fatalf("records = %+v", records)
	}
}

func TestSupersededRejectionNoLongerBlocksReview(t *testing.T) {
	verifier, gates, reviews := setupReview(t)
	old := rejection("seat-a", "d-old", 1)
	superseded := time.Now()
	old.SupersededAt = &superseded
	reviews.decisions = []*workflowmodels.WorkflowStepDecision{old, approval("human-seat", "user", "default", "pr-1@h1")}
	verifier.DecisionRecorded(context.Background(), "task-1", "review", "approved")
	if records := gates.records(); len(records) != 1 || records[0].Result != models.TaskCompletionResultPass {
		t.Fatalf("records = %+v", records)
	}
}

func TestRouterCountsAReturnToWorkOnlyForARejection(t *testing.T) {
	router, gates, _, _ := setupRouter(t, "work", reviewCriterion())
	reviews := router.review.reviews.(*fakeReviews)
	approved := approval("seat-a", "agent", "qa", "pr-1@h2")
	approved.DecidedAt = time.Date(2026, 11, 21, 10, 0, 0, 0, time.UTC)
	reviews.decisions = []*workflowmodels.WorkflowStepDecision{approved}
	router.OnTaskMoved(context.Background(), "task-1", "review", "manual-1")
	if got := gates.snapshots["task-1"].Criteria[0].ReworkCount; got != 0 {
		t.Fatalf("manual move after an approval counted: %d", got)
	}
	reviews.decisions = append(reviews.decisions, rejection("seat-a", "d-rej", 5))
	router.OnTaskMoved(context.Background(), "task-1", "review", "transition-2")
	router.OnTaskMoved(context.Background(), "task-1", "review", "transition-3")
	if got := gates.snapshots["task-1"].Criteria[0].ReworkCount; got != 1 {
		t.Fatalf("one rejection counted %d times", got)
	}
}

func TestExtendingTheCapAppliesTheHeldReturn(t *testing.T) {
	svc, gates := newControlService(t, failedCI("pr-1@h2"))
	navigator := &fakeNavigator{placement: officePlacement("approval")}
	svc.SetNavigator(navigator)
	svc.Router.SetCapResolver(func(context.Context, string, models.TaskCompletionCriterion) (int, int) { return 3, 0 })
	held := failedCI("pr-1@h2")
	held.OwnerKind, held.OwnerID, held.ReworkCount = ownerKindAgent, "owner", 1
	gates.setCriteria("task-1", held)
	svc.Router.SetCapResolver(func(context.Context, string, models.TaskCompletionCriterion) (int, int) { return 3, 1 })
	svc.Router.RouteCriterion(context.Background(), "task-1", held)
	if criterion := gates.snapshots["task-1"].Criteria[0]; criterion.HeldReason != HoldReworkCapReached || len(navigator.moves) != 0 {
		t.Fatalf("at cap: criterion=%+v moves=%v", criterion, navigator.moves)
	}
	svc.Router.SetCapResolver(func(context.Context, string, models.TaskCompletionCriterion) (int, int) { return 3, 2 })
	gates.snapshots["task-1"].Criteria[0].HeldReason = ""
	svc.Router.RouteCriterion(context.Background(), "task-1", gates.snapshots["task-1"].Criteria[0])
	if criterion := gates.snapshots["task-1"].Criteria[0]; criterion.ReworkCount != 2 || len(navigator.moves) != 1 || navigator.moves[0] != "work" {
		t.Fatalf("after extension: criterion=%+v moves=%v", criterion, navigator.moves)
	}
}

type flakyRuns struct {
	failures int
	agents   []string
}

func (f *flakyRuns) QueueRun(_ context.Context, agentID, _, _, _ string) error {
	if f.failures > 0 {
		f.failures--
		return errors.New("queue unavailable")
	}
	f.agents = append(f.agents, agentID)
	return nil
}

func TestFailedEscalationIsRetriedBySweepAndMarkedOnce(t *testing.T) {
	gates := newFakeGates()
	heldAt := time.Now()
	held := ownedCriterion(ownerKindAgent, "owner")
	held.HeldAt = &heldAt
	gates.setCriteria("task-1", held)
	gates.heldUnescalated = []string{"task-1"}
	runs := &flakyRuns{failures: 1}
	escalator := NewEscalator(gates, fakeTasks{assignee: "worker"}, fakeAgents{ceo: "ceo"}, runs, nil, newTestLogger(t))
	escalator.EscalateHold(context.Background(), "task-1", held)
	if gates.snapshots["task-1"].Criteria[0].EscalatedAt != nil {
		t.Fatal("failed escalation was marked")
	}
	if err := escalator.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if gates.snapshots["task-1"].Criteria[0].EscalatedAt == nil || len(runs.agents) != 1 {
		t.Fatalf("sweep did not escalate: runs=%v", runs.agents)
	}
	if err := escalator.sweep(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(runs.agents) != 1 {
		t.Fatalf("escalated twice: %v", runs.agents)
	}
}

func TestSendBackRequiresAReasonAndUsesTheProtectedAction(t *testing.T) {
	svc, gates := newControlService(t, ciCriterion())
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1"})
	request := ProtectedPathsAction{Action: ProtectedPathsSendBack, ExpectedCriterionRevision: 1, ExpectedSubjectRevision: "pr-1@h1"}
	if _, err := svc.DecideProtectedPaths(ctx, "task-1", "ci", request); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("blank reason err = %v", err)
	}
	request.Reason = "tests were weakened"
	if _, err := svc.DecideProtectedPaths(ctx, "task-1", "ci", request); err != nil {
		t.Fatal(err)
	}
	if last := gates.controls[len(gates.controls)-1]; last.Action != models.TaskCompletionControlSendBackProtected || last.SubjectRevision != "pr-1@h1" {
		t.Fatalf("control = %+v", last)
	}
}

func TestUnreadableChangedFilesCannotClearProtectedPaths(t *testing.T) {
	verifier, gates, prs := setupCI(t, "tests/**")
	prs.checks["pr-1"] = CheckObservation{HeadSHA: "h1", State: checkStateSuccess, Observed: 1}
	prs.filesErr = errors.New("rate limited")
	if err := verifier.Evaluate(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	records := gates.records()
	if len(records) != 1 || records[0].Result != models.TaskCompletionResultCannotEvaluate ||
		records[0].Reason != ReasonChangedFilesIncomplete || records[0].SubjectRevision != "pr-1@h1" {
		t.Fatalf("records = %+v", records)
	}
}
