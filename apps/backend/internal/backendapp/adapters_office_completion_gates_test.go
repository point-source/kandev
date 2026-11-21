package backendapp

import (
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
