package completiongates

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-OFFICE-COMPLETION-GATES-008.9
func TestAHeldReviewFollowsTheHeadsSoAPersonCanStillAct(t *testing.T) {
	svc, gates, runs := newReviewerService(t, reviewCriterion(), 0)
	svc.ReviewerCast(context.Background(), "task-1", "worker", false)
	escalations := len(runs.agents)
	svc.Router.prs.(*fakePRs).links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h2"}}

	svc.Router.OnHeadsChanged(context.Background(), "task-1")

	review := reviewState(gates)
	if review.HoldKind != HoldNoReviewerAvailable || review.ResultSubjectRevision != "pr-1@h2" ||
		len(review.ResultDetail) != 1 || review.ResultDetail[0] != CauseAssigneeOnly {
		t.Fatalf("held review after a push = %+v", review)
	}
	if len(runs.agents) != escalations {
		t.Fatalf("moving the heads escalated the same hold again: %+v", runs)
	}
	last := gates.controls[len(gates.controls)-1]
	if last.Action != models.TaskCompletionControlHold || last.SubjectRevision != "pr-1@h2" {
		t.Fatalf("rebind control = %+v", last)
	}
}

// A person's verdict belongs to the commit it was given at; when the heads
// move under a hold it caused, the review waits for a verdict at the new
// heads instead of carrying the old one forward.
//
// @covers AC-OFFICE-COMPLETION-GATES-004.6
func TestAPushUnderASendBackHoldDropsTheOldVerdict(t *testing.T) {
	sentBack := reviewCriterion()
	sentBack.Result, sentBack.ResultReason, sentBack.ResultDetail = models.TaskCompletionResultFail, models.TaskCompletionReasonSentBack, []string{"missed a case"}
	sentBack.ResultSubjectRevision, sentBack.HoldKind, sentBack.ReworkCount = "pr-1@h1", HoldReworkLimitReached, 3
	svc, gates, _ := newReviewerService(t, sentBack, 0)
	svc.Router.prs.(*fakePRs).links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h2"}}

	svc.Router.OnHeadsChanged(context.Background(), "task-1")

	records := gates.records()
	if len(records) != 1 || records[0].SubjectRevision != "pr-1@h2" || records[0].Result != models.TaskCompletionResultPending ||
		records[0].Reason != reasonHeadMoved || records[0].ReleaseHold != "" || records[0].Hold != "" {
		t.Fatalf("records = %+v", records)
	}
	for _, control := range gates.controls {
		if control.Action == models.TaskCompletionControlHold {
			t.Fatalf("the old verdict's hold was rebound as is: %+v", control)
		}
	}
}
