package backendapp

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

type seaterTasks struct{ task models.Task }

func (s seaterTasks) GetTask(context.Context, string) (*models.Task, error) { return &s.task, nil }

type seaterSteps struct{}

func (seaterSteps) ListStepsByWorkflow(context.Context, string) ([]*workflowmodels.WorkflowStep, error) {
	return []*workflowmodels.WorkflowStep{
		{ID: "work", Position: 1, StageType: workflowmodels.StageTypeWork},
		{ID: "review", Position: 2, StageType: workflowmodels.StageTypeReview},
	}, nil
}

type seaterCaster struct {
	cast engine.ParticipantSeatCastResult
}

func (s seaterCaster) CastParticipantSeat(context.Context, string, string, string, string) (engine.ParticipantSeatCastResult, error) {
	return s.cast, nil
}

type seaterAdds struct{ added []string }

func (s *seaterAdds) AddTaskReviewer(_ context.Context, _, _, agentID string) error {
	s.added = append(s.added, agentID)
	return nil
}

type seaterRuns struct{ agents, payloads []string }

func (s *seaterRuns) QueueRun(_ context.Context, agentID, _, payload, _ string) error {
	s.agents, s.payloads = append(s.agents, agentID), append(s.payloads, payload)
	return nil
}

func newSeater(cast engine.ParticipantSeatCastResult, step string) (completionGateReviewerSeater, *seaterAdds, *seaterRuns) {
	adds, runs := &seaterAdds{}, &seaterRuns{}
	return completionGateReviewerSeater{
		tasks: seaterTasks{task: models.Task{ID: "t1", WorkflowID: "wf", WorkflowStepID: step, AssigneeAgentProfileID: "worker"}},
		steps: seaterSteps{}, caster: seaterCaster{cast: cast}, reviewers: adds, runs: runs,
	}, adds, runs
}

// @covers AC-OFFICE-COMPLETION-GATES-009.4
func TestReviewerSeaterSeatsAndWakesANewReviewer(t *testing.T) {
	seater, adds, runs := newSeater(engine.ParticipantSeatCastResult{AgentProfileID: "qa"}, "review")
	seated, err := seater.SeatReviewer(context.Background(), "t1")
	if err != nil || !seated || len(adds.added) != 1 || adds.added[0] != "qa" || len(runs.agents) != 1 {
		t.Fatalf("seated=%v err=%v adds=%v runs=%v", seated, err, adds.added, runs.agents)
	}
	if runs.payloads[0] != `{"task_id":"t1","workflow_step_id":"review"}` {
		t.Fatalf("payload = %s", runs.payloads[0])
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-009.4
func TestReviewerSeaterLeavesTheHoldWithoutANewReviewer(t *testing.T) {
	cases := map[string]struct {
		cast engine.ParticipantSeatCastResult
		step string
	}{
		"still the assignee": {engine.ParticipantSeatCastResult{AgentProfileID: "worker"}, "review"},
		"still unfillable":   {engine.ParticipantSeatCastResult{Unfillable: true}, "review"},
		"not in review":      {engine.ParticipantSeatCastResult{AgentProfileID: "qa"}, "work"},
	}
	for name, tc := range cases {
		seater, adds, runs := newSeater(tc.cast, tc.step)
		if seated, err := seater.SeatReviewer(context.Background(), "t1"); err != nil || seated || len(adds.added) != 0 || len(runs.agents) != 0 {
			t.Errorf("%s: seated=%v err=%v adds=%v runs=%v", name, seated, err, adds.added, runs.agents)
		}
	}
}
