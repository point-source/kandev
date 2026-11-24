package backendapp

import (
	"context"
	"testing"

	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

func TestCompletingStepIDsAreFinalCompletingSteps(t *testing.T) {
	steps := []*workflowmodels.WorkflowStep{
		{ID: "work", Position: 1, CompleteTaskOnEnter: true}, {ID: "approval", Position: 2}, {ID: "done", Position: 3, CompleteTaskOnEnter: true},
	}
	if got := completingStepIDs(steps); len(got) != 1 || !got["done"] {
		t.Fatalf("completing steps = %v", got)
	}
	steps[2].CompleteTaskOnEnter = false
	if got := completingStepIDs(steps); len(got) != 0 {
		t.Fatalf("non-completing final step = %v", got)
	}
}

func TestFailedTasksAreStillRouted(t *testing.T) {
	if placementTerminal(v1.TaskStateFailed) || !placementTerminal(v1.TaskStateCompleted) || !placementTerminal(v1.TaskStateCancelled) {
		t.Fatal("only completed and cancelled tasks are terminal for routing")
	}
}

type stepList []*workflowmodels.WorkflowStep

func (s stepList) ListStepsByWorkflow(context.Context, string) ([]*workflowmodels.WorkflowStep, error) {
	return s, nil
}

// @covers AC-OFFICE-COMPLETION-GATES-010.1
func TestHasStepAfterWorkNeedsANonCompletingStepAfterWork(t *testing.T) {
	work := &workflowmodels.WorkflowStep{ID: "work", Position: 1, StageType: workflowmodels.StageTypeWork}
	done := &workflowmodels.WorkflowStep{ID: "done", Position: 3, CompleteTaskOnEnter: true}
	backlog := &workflowmodels.WorkflowStep{ID: "backlog", Position: 0}
	review := &workflowmodels.WorkflowStep{ID: "review", Position: 2, StageType: workflowmodels.StageTypeReview}
	for name, tc := range map[string]struct {
		steps stepList
		want  bool
	}{
		"work then done":         {stepList{done, work, backlog}, false},
		"work, review then done": {stepList{done, review, work}, true},
		"no work stage":          {stepList{backlog, done}, true},
	} {
		got, err := completionGateStepLimits{steps: tc.steps}.HasStepAfterWork(context.Background(), "wf")
		if err != nil || got != tc.want {
			t.Errorf("%s: got %v, %v", name, got, err)
		}
	}
}
