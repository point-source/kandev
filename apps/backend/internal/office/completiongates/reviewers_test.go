package completiongates

import (
	"context"
	"testing"
	"time"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

func newReviewerService(t *testing.T, review models.TaskCompletionCriterion, waitMinutes int) (*Service, *fakeGates, *payloadRuns) {
	t.Helper()
	gates, runs, prs := newFakeGates(), &payloadRuns{}, newFakePRs()
	gates.setCriteria("task-1", review)
	prs.links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h1"}}
	svc := NewService(Dependencies{
		Gates: gates, PRs: prs, Tasks: fakeTasks{assignee: "worker"}, Reviews: &fakeReviews{},
		Settings: &fakeSettings{settings: officemodels.CompletionGateSettings{DefaultReviewWaitLimit: waitMinutes}},
		Agents:   fakeAgents{ceo: "ceo"}, Runs: runs, Held: fakeHeld{}, Logger: newTestLogger(t),
	})
	svc.SetNavigator(&fakeNavigator{placement: officePlacement("review")})
	return svc, gates, runs
}

func reviewState(gates *fakeGates) models.TaskCompletionCriterion {
	gates.mu.Lock()
	defer gates.mu.Unlock()
	return gates.snapshots["task-1"].Criteria[0]
}

// @covers AC-OFFICE-COMPLETION-GATES-009.2
func TestAnUnfillableReviewerSeatHoldsAtOnceAndEscalates(t *testing.T) {
	agentOwned := reviewCriterion()
	agentOwned.OwnerKind, agentOwned.OwnerID = ownerKindAgent, "ceo"
	svc, gates, runs := newReviewerService(t, agentOwned, 0)
	svc.ReviewerCast(context.Background(), "task-1", "worker", false)
	review := reviewState(gates)
	if review.HoldKind != HoldNoReviewerAvailable || len(review.ResultDetail) != 1 || review.ResultDetail[0] != CauseAssigneeOnly {
		t.Fatalf("review = %+v", review)
	}
	if hold := gates.controls[0]; hold.Action != models.TaskCompletionControlHold || hold.SubjectRevision != "pr-1@h1" {
		t.Fatalf("hold = %+v", hold)
	}
	if len(runs.agents) != 1 || runs.reasons[0] != RunReasonCompletionGateHeld {
		t.Fatalf("escalation runs = %+v", runs)
	}
	svc.ReviewerCast(context.Background(), "task-1", "", true)
	if review := reviewState(gates); review.ResultDetail[0] != CauseNoReviewerRole {
		t.Fatalf("unfillable cause = %v", review.ResultDetail)
	}
	svc.ReviewerCast(context.Background(), "task-1", "qa", false)
	if review := reviewState(gates); review.HoldKind != "" {
		t.Fatalf("a fillable seat left the hold: %+v", review)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-009.2
func TestOnlyTheAuthorsProviderHoldsAtOnce(t *testing.T) {
	svc, gates, _ := newReviewerService(t, reviewCriterion(), 0)
	svc.ReviewSeatParked(context.Background(), "task-1", true, "", time.Time{})
	if review := reviewState(gates); review.HoldKind != HoldNoReviewerAvailable || review.ResultDetail[0] != CauseAuthorProviderOnly {
		t.Fatalf("review = %+v", review)
	}
	svc.ReviewSeatLaunched(context.Background(), "task-1")
	if review := reviewState(gates); review.HoldKind != "" {
		t.Fatalf("launch left the hold: %+v", review)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-009.3
func TestATemporaryParkWaitsIndefinitelyWithoutAWaitLimit(t *testing.T) {
	svc, gates, _ := newReviewerService(t, reviewCriterion(), 0)
	retry := time.Date(2026, 11, 21, 15, 0, 0, 0, time.UTC)
	svc.ReviewSeatParked(context.Background(), "task-1", false, "waiting_for_capacity", retry)
	records := gates.records()
	if len(records) != 1 || records[0].Result != models.TaskCompletionResultPending || records[0].Reason != ReasonWaitingForReviewer ||
		records[0].Detail[0] != "waiting_for_capacity" || records[0].Detail[1] != retry.Format(time.RFC3339) {
		t.Fatalf("records = %+v", records)
	}
	waiting := reviewState(gates)
	waiting.Result, waiting.ResultReason, waiting.ResultDetail = models.TaskCompletionResultPending, ReasonWaitingForReviewer, records[0].Detail
	gates.setCriteria("task-1", waiting)
	gates.unmet = []string{"task-1"}
	svc.sweepReviewWaits(context.Background(), time.Now().Add(1000*time.Hour))
	if review := reviewState(gates); review.HoldKind != "" {
		t.Fatalf("a park without a wait limit held: %+v", review)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-009.3
func TestAParkThatOutlastsTheWaitLimitHolds(t *testing.T) {
	svc, gates, _ := newReviewerService(t, reviewCriterion(), 30)
	svc.ReviewSeatParked(context.Background(), "task-1", false, "waiting_for_capacity", time.Time{})
	waiting := reviewState(gates)
	waiting.Result, waiting.ResultReason, waiting.ResultDetail = models.TaskCompletionResultPending, ReasonWaitingForReviewer, gates.records()[0].Detail
	gates.setCriteria("task-1", waiting)
	gates.unmet = []string{"task-1"}
	svc.sweepReviewWaits(context.Background(), time.Now().Add(10*time.Minute))
	if review := reviewState(gates); review.HoldKind != "" {
		t.Fatalf("held before the limit: %+v", review)
	}
	svc.sweepReviewWaits(context.Background(), time.Now().Add(31*time.Minute))
	if review := reviewState(gates); review.HoldKind != HoldReviewWaitLimitReached {
		t.Fatalf("review = %+v", review)
	}
	svc.ReviewSeatLaunched(context.Background(), "task-1")
	if review := reviewState(gates); review.HoldKind != "" {
		t.Fatalf("launch left the wait hold: %+v", review)
	}
}
