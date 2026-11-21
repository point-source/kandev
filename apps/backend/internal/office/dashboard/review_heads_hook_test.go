package dashboard_test

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/dashboard"
	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

type spyReviewHeads struct {
	heads, notice string
	recorded      []string
}

func (s *spyReviewHeads) DecisionHeads(context.Context, string, string, string) (string, string) {
	return s.heads, s.notice
}

func (s *spyReviewHeads) DecisionRecorded(_ context.Context, taskID, _, decision string) {
	s.recorded = append(s.recorded, taskID+":"+decision)
}

// @covers AC-OFFICE-COMPLETION-GATES-004.1, AC-OFFICE-COMPLETION-GATES-005.2
func TestRecordAgentDecisionPinsHeadsAndReturnsNotice(t *testing.T) {
	deps := newTestDeps(t)
	insertTestTask(t, deps.db, "rh1", "ws-d", "RH1", "in_review", 2)
	mustAddParticipant(t, deps, "rh1", "agent-1", models.ParticipantRoleReviewer)
	spy := &spyDecisionDispatcher{role: models.ParticipantRoleReviewer, participantID: "participant-1"}
	deps.svc.SetWorkflowEngineDispatcher(spy)
	hook := &spyReviewHeads{heads: "pr-1@h1", notice: "review_self_approval_ignored"}
	deps.svc.SetReviewHeadsHook(hook)

	result, err := deps.svc.RecordAgentDecision(context.Background(), dashboard.RecordAgentDecisionInput{
		TaskID: "rh1", AgentProfileID: "agent-1", Decision: engine.DecisionApproved, Reason: "lgtm",
	})
	if err != nil {
		t.Fatalf("RecordAgentDecision: %v", err)
	}
	if spy.lastInput.PRHeads != "pr-1@h1" || result.Notice != "review_self_approval_ignored" {
		t.Fatalf("heads=%q notice=%q", spy.lastInput.PRHeads, result.Notice)
	}
	if len(hook.recorded) != 1 || hook.recorded[0] != "rh1:approved" {
		t.Fatalf("recorded = %v", hook.recorded)
	}
}
