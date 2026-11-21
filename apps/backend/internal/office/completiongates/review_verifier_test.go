package completiongates

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

type fakeTasks struct{ assignee string }

func (f fakeTasks) GetTask(_ context.Context, taskID string) (*models.Task, error) {
	return &models.Task{ID: taskID, WorkspaceID: "ws-1", WorkflowID: "wf-1", AssigneeAgentProfileID: f.assignee}, nil
}

type fakeReviews struct {
	seats     []*workflowmodels.WorkflowStepParticipant
	decisions []*workflowmodels.WorkflowStepDecision
}

func (f *fakeReviews) ListStepDecisions(context.Context, string, string) ([]*workflowmodels.WorkflowStepDecision, error) {
	return f.decisions, nil
}

func (f *fakeReviews) ListStepParticipantsForTask(context.Context, string, string) ([]*workflowmodels.WorkflowStepParticipant, error) {
	return f.seats, nil
}

func reviewCriterion() models.TaskCompletionCriterion {
	return models.TaskCompletionCriterion{ID: "review", Kind: models.TaskCompletionCriterionReviewAtHead, CriterionRevision: 1}
}

func reviewerSeat(id, agent string) *workflowmodels.WorkflowStepParticipant {
	return &workflowmodels.WorkflowStepParticipant{ID: id, Role: workflowmodels.ParticipantRoleReviewer, AgentProfileID: agent, DecisionRequired: true}
}

func approval(participant, deciderType, decider, heads string) *workflowmodels.WorkflowStepDecision {
	return &workflowmodels.WorkflowStepDecision{ID: "d-" + participant + decider, ParticipantID: participant, Decision: "approved",
		DeciderType: deciderType, DeciderID: decider, Role: "reviewer", PRHeads: heads}
}

func setupReview(t *testing.T) (*ReviewVerifier, *fakeGates, *fakeReviews) {
	t.Helper()
	gates, prs := newFakeGates(), newFakePRs()
	gates.setCriteria("task-1", reviewCriterion())
	prs.links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h1"}}
	reviews := &fakeReviews{seats: []*workflowmodels.WorkflowStepParticipant{reviewerSeat("seat-a", "qa"), reviewerSeat("seat-b", "sec")}}
	return NewReviewVerifier(gates, prs, fakeTasks{assignee: "worker"}, reviews, newTestLogger(t)), gates, reviews
}

// @covers AC-OFFICE-COMPLETION-GATES-004.1, AC-OFFICE-COMPLETION-GATES-005.2
func TestReviewDecisionHeads(t *testing.T) {
	verifier, _, _ := setupReview(t)
	if heads, notice := verifier.DecisionHeads(context.Background(), "task-1", "agent", "qa"); heads != "pr-1@h1" || notice != "" {
		t.Fatalf("reviewer heads=%q notice=%q", heads, notice)
	}
	if heads, notice := verifier.DecisionHeads(context.Background(), "task-1", "agent", "worker"); heads != "" || notice != NoticeSelfApprovalIgnored {
		t.Fatalf("assignee heads=%q notice=%q", heads, notice)
	}
	if heads, _ := verifier.DecisionHeads(context.Background(), "task-without-criterion", "agent", "qa"); heads != "" {
		t.Fatalf("ungated heads=%q", heads)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-004.2
func TestReviewPassesWhenEverySeatApprovedTheCurrentHeads(t *testing.T) {
	verifier, gates, reviews := setupReview(t)
	reviews.decisions = []*workflowmodels.WorkflowStepDecision{approval("seat-a", "agent", "qa", "pr-1@h1")}
	verifier.DecisionRecorded(context.Background(), "task-1", "review", "approved")
	if records := gates.records(); len(records) != 0 {
		t.Fatalf("partial approval recorded: %+v", records)
	}
	reviews.decisions = append(reviews.decisions, approval("seat-b", "agent", "sec", "pr-1@h0"))
	verifier.DecisionRecorded(context.Background(), "task-1", "review", "approved")
	if records := gates.records(); len(records) != 0 {
		t.Fatalf("approval of an old head counted: %+v", records)
	}
	reviews.decisions[1] = approval("seat-b", "agent", "sec", "pr-1@h1")
	verifier.DecisionRecorded(context.Background(), "task-1", "review", "approved")
	records := gates.records()
	if len(records) != 1 || records[0].Result != models.TaskCompletionResultPass || records[0].SubjectRevision != "pr-1@h1" ||
		records[0].VerifierKind != VerifierKindReviews || records[0].VerifierID != "sec" {
		t.Fatalf("records = %+v", records)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-004.4
func TestHumanApprovalCountsAtItsHeads(t *testing.T) {
	verifier, gates, reviews := setupReview(t)
	reviews.decisions = []*workflowmodels.WorkflowStepDecision{approval("human-seat", "user", "default", "pr-1@h1")}
	verifier.DecisionRecorded(context.Background(), "task-1", "review", "approved")
	if records := gates.records(); len(records) != 1 || records[0].Result != models.TaskCompletionResultPass {
		t.Fatalf("records = %+v", records)
	}
}

func TestRejectionRecordsFail(t *testing.T) {
	verifier, gates, reviews := setupReview(t)
	reviews.decisions = []*workflowmodels.WorkflowStepDecision{rejection("seat-a", "d-1", 1)}
	verifier.DecisionRecorded(context.Background(), "task-1", "review", "rejected")
	if records := gates.records(); len(records) != 1 || records[0].Result != models.TaskCompletionResultFail || records[0].Reason != ReasonReviewRejected {
		t.Fatalf("records = %+v", records)
	}
}
