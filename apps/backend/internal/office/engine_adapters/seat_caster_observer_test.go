package engine_adapters

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/office/repository/sqlite"
)

type spyReviewerObserver struct {
	agents     []string
	unfillable []bool
}

func (s *spyReviewerObserver) ReviewerCast(_ context.Context, _ string, agentProfileID string, unfillable bool) {
	s.agents, s.unfillable = append(s.agents, agentProfileID), append(s.unfillable, unfillable)
}

// @covers AC-OFFICE-COMPLETION-GATES-009.2
func TestSeatCasterReportsReviewerSeatsOnly(t *testing.T) {
	office := &fakeOfficeRepo{fields: &sqlite.TaskExecutionFields{ID: "t-1", WorkspaceID: "ws-1"}}
	observer := &spyReviewerObserver{}
	adapter := NewSeatCasterAdapter(office, &fakeSeatCasterWorkflowRepo{runner: "runner-agent"})
	adapter.Observer = observer
	if _, err := adapter.CastParticipantSeat(context.Background(), "wf-1", "t-1", "review", "reviewer"); err != nil {
		t.Fatal(err)
	}
	if _, err := adapter.CastParticipantSeat(context.Background(), "wf-1", "t-1", "approval", "approver"); err != nil {
		t.Fatal(err)
	}
	if len(observer.agents) != 1 || observer.agents[0] != "runner-agent" || observer.unfillable[0] {
		t.Fatalf("observer = %+v", observer)
	}
	adapter.Workflow = &fakeSeatCasterWorkflowRepo{}
	if _, err := adapter.CastParticipantSeat(context.Background(), "wf-1", "t-1", "review", "reviewer"); err != nil {
		t.Fatal(err)
	}
	if len(observer.unfillable) != 2 || !observer.unfillable[1] {
		t.Fatalf("unfillable not reported: %+v", observer)
	}
}
