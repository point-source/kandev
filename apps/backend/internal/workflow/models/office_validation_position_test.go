package models

import "testing"

// A move_to_next out of a step that shares its position with the completing
// step goes nowhere at runtime, so validation must not count it.
func TestValidateOfficeWorkflowMoveToNextRequiresHigherPosition(t *testing.T) {
	steps := []*WorkflowStep{
		{
			ID: "work", Name: "Work", Position: 0, StageType: StageTypeWork, IsStartStep: true,
			AutoAdvanceRequiresSignal: true,
			Events:                    StepEvents{OnTurnComplete: []OnTurnCompleteAction{{Type: OnTurnCompleteMoveToNext}}},
		},
		{ID: "done", Name: "Done", Position: 0, StageType: StageTypeCustom, CompleteTaskOnEnter: true},
	}
	problems := ValidateOfficeWorkflow(steps, allProfilesExist)
	if len(problems) != 1 || problems[0].Code != OfficeProblemCompletingStepUnreachable {
		t.Fatalf("problems = %+v, want completing_step_unreachable", problems)
	}

	steps[1].Position = 1
	if problems := ValidateOfficeWorkflow(steps, allProfilesExist); len(problems) != 0 {
		t.Fatalf("problems = %+v, want none once positions differ", problems)
	}
}
