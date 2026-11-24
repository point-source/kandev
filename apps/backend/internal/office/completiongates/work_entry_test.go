package completiongates

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

func reviewAtLimit(t *testing.T, router *Router, gates *fakeGates) {
	t.Helper()
	review := reviewCriterion()
	review.ReworkCount = 1
	gates.setCriteria("task-1", review)
	router.SetLimitResolver(func(context.Context, string, models.TaskCompletionCriterion) (int, int) { return 3, 1 })
	router.review.reviews.(*fakeReviews).decisions = []*workflowmodels.WorkflowStepDecision{rejection("seat-a", "d-1", 1)}
}

// @covers AC-OFFICE-COMPLETION-GATES-004.5, AC-OFFICE-COMPLETION-GATES-007.3
func TestARejectionAtTheLimitHoldsBeforeTheWorkerStarts(t *testing.T) {
	router, gates, _, escalator := setupRouter(t, "work", reviewCriterion())
	reviewAtLimit(t, router, gates)

	router.SettleWorkEntry(context.Background(), "task-1")
	review := gates.snapshots["task-1"].Criteria[0]
	if review.HoldKind != HoldReworkLimitReached || review.ReworkCount != 1 || len(escalator.held) != 1 {
		t.Fatalf("review = %+v escalations = %v", review, escalator.held)
	}
	router.OnTaskMoved(context.Background(), "task-1", "review", "42")
	router.SettleWorkEntry(context.Background(), "task-1")
	if len(escalator.held) != 1 {
		t.Fatalf("the hold escalated %d times", len(escalator.held))
	}
}

func TestARejectionBelowTheLimitCountsOnceAcrossStartAndMove(t *testing.T) {
	router, gates, navigator, escalator := setupRouter(t, "work", reviewCriterion())
	router.review.reviews.(*fakeReviews).decisions = []*workflowmodels.WorkflowStepDecision{rejection("seat-a", "d-1", 1)}

	router.SettleWorkEntry(context.Background(), "task-1")
	router.OnTaskMoved(context.Background(), "task-1", "review", "42")
	review := gates.snapshots["task-1"].Criteria[0]
	if review.ReworkCount != 1 || review.HoldKind != "" || len(navigator.moves) != 0 || len(escalator.held) != 0 {
		t.Fatalf("review = %+v moves = %v escalations = %v", review, navigator.moves, escalator.held)
	}
}

func TestSettleWorkEntryIgnoresTasksOutsideWorkAndApprovals(t *testing.T) {
	router, gates, _, _ := setupRouter(t, "review", reviewCriterion())
	reviewAtLimit(t, router, gates)
	router.SettleWorkEntry(context.Background(), "task-1")
	if held := gates.snapshots["task-1"].Criteria[0].HoldKind; held != "" {
		t.Fatalf("a task in review was held: %q", held)
	}

	router, gates, _, _ = setupRouter(t, "work", reviewCriterion())
	reviewAtLimit(t, router, gates)
	router.review.reviews.(*fakeReviews).decisions = []*workflowmodels.WorkflowStepDecision{approval("seat-a", "agent", "qa", "pr-1@h2")}
	router.SettleWorkEntry(context.Background(), "task-1")
	if held := gates.snapshots["task-1"].Criteria[0].HoldKind; held != "" {
		t.Fatalf("an approval was counted as a rejection: %q", held)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-007.3
func TestAssigneeStartIsHeldByARejectionAtTheLimit(t *testing.T) {
	svc, gates := newControlService(t, reviewCriterion())
	svc.SetNavigator(&fakeNavigator{placement: officePlacement("work")})
	reviewAtLimit(t, svc.Router, gates)

	if svc.AssigneeStartHeld(context.Background(), "task-1", "reviewer") {
		t.Fatal("a non-assignee start was held")
	}
	if !svc.AssigneeStartHeld(context.Background(), "task-1", "worker") {
		t.Fatal("the assignee started past the rework limit")
	}
}
