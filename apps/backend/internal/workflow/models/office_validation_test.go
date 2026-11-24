package models

import (
	"reflect"
	"testing"
)

func officeSeatOnEnter(role string) []OnEnterAction {
	return []OnEnterAction{
		{Type: OnEnterClearDecisions},
		{Type: OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": role}},
		{Type: OnEnterQueueRunForEachParticipant, Config: map[string]interface{}{"role": role}},
	}
}

func moveTo(stepID string) []OnTurnCompleteAction {
	return []OnTurnCompleteAction{{Type: OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": stepID}}}
}

// validOfficeSteps mirrors the shape of the built-in Office Default workflow.
func validOfficeSteps() []*WorkflowStep {
	return []*WorkflowStep{
		{ID: "backlog", Name: "Backlog", Position: 0, StageType: StageTypeCustom},
		{
			ID: "work", Name: "Work", Position: 1, StageType: StageTypeWork, IsStartStep: true,
			AutoAdvanceRequiresSignal: true, Events: StepEvents{OnTurnComplete: moveTo("review")},
		},
		{
			ID: "review", Name: "Review", Position: 2, StageType: StageTypeReview,
			Events: StepEvents{OnEnter: officeSeatOnEnter("reviewer"), OnTurnComplete: moveTo("approval")},
		},
		{
			ID: "approval", Name: "Approval", Position: 3, StageType: StageTypeApproval,
			Events: StepEvents{OnEnter: officeSeatOnEnter("approver"), OnTurnComplete: moveTo("done")},
		},
		{ID: "done", Name: "Done", Position: 4, StageType: StageTypeCustom, CompleteTaskOnEnter: true},
	}
}

func stepByID(steps []*WorkflowStep, id string) *WorkflowStep {
	for _, s := range steps {
		if s.ID == id {
			return s
		}
	}
	return nil
}

func allProfilesExist(string) bool { return true }

func TestValidateOfficeWorkflowAcceptsValidWorkflow(t *testing.T) {
	if problems := ValidateOfficeWorkflow(validOfficeSteps(), allProfilesExist); len(problems) != 0 {
		t.Fatalf("problems = %+v, want none", problems)
	}
}

func TestValidateOfficeWorkflowRules(t *testing.T) {
	cases := []struct {
		name   string
		mutate func([]*WorkflowStep) []*WorkflowStep
		exists func(string) bool
		want   []OfficeWorkflowProblem
	}{
		{
			name: "AC-005.1 no start step",
			mutate: func(s []*WorkflowStep) []*WorkflowStep {
				stepByID(s, "work").IsStartStep = false
				return s
			},
			want: []OfficeWorkflowProblem{{Code: OfficeProblemStartStepCount, Count: 0}},
		},
		{
			name: "AC-005.1 two start steps",
			mutate: func(s []*WorkflowStep) []*WorkflowStep {
				stepByID(s, "backlog").IsStartStep = true
				return s
			},
			want: []OfficeWorkflowProblem{{Code: OfficeProblemStartStepCount, Count: 2}},
		},
		{
			name: "AC-005.2 work stage without completion signal",
			mutate: func(s []*WorkflowStep) []*WorkflowStep {
				stepByID(s, "work").AutoAdvanceRequiresSignal = false
				return s
			},
			want: []OfficeWorkflowProblem{{Code: OfficeProblemMissingWorkStage}},
		},
		{
			name: "AC-005.3 final step does not complete",
			mutate: func(s []*WorkflowStep) []*WorkflowStep {
				stepByID(s, "done").CompleteTaskOnEnter = false
				return s
			},
			want: []OfficeWorkflowProblem{{Code: OfficeProblemFinalStepNotCompleting, StepName: "Done"}},
		},
		{
			name: "AC-005.3 completing step unreachable",
			mutate: func(s []*WorkflowStep) []*WorkflowStep {
				stepByID(s, "approval").Events.OnTurnComplete = nil
				return s
			},
			want: []OfficeWorkflowProblem{{Code: OfficeProblemCompletingStepUnreachable, StepName: "Done"}},
		},
		{
			name: "AC-005.4 review step missing seat",
			mutate: func(s []*WorkflowStep) []*WorkflowStep {
				review := stepByID(s, "review")
				review.Events.OnEnter = review.Events.OnEnter[2:]
				return s
			},
			want: []OfficeWorkflowProblem{{
				Code: OfficeProblemMissingSeatAction, StepName: "Review", Role: "reviewer",
				Action: string(OnEnterEnsureParticipantSeat),
			}},
		},
		{
			name: "AC-005.4 approval step with no seat actions at all",
			mutate: func(s []*WorkflowStep) []*WorkflowStep {
				stepByID(s, "approval").Events.OnEnter = nil
				return s
			},
			want: []OfficeWorkflowProblem{
				{Code: OfficeProblemMissingSeatAction, StepName: "Approval", Role: "approver", Action: string(OnEnterEnsureParticipantSeat)},
				{Code: OfficeProblemMissingSeatAction, StepName: "Approval", Role: "approver", Action: string(OnEnterQueueRunForEachParticipant)},
			},
		},
		{
			name: "AC-005.5 unknown agent profile",
			mutate: func(s []*WorkflowStep) []*WorkflowStep {
				review := stepByID(s, "review")
				review.Events.OnEnter = append(review.Events.OnEnter, OnEnterAction{
					Type: OnEnterRunCodeReview, Config: map[string]interface{}{ReviewAgentProfileConfigKey: "gone"},
				})
				return s
			},
			exists: func(id string) bool { return id != "gone" },
			want: []OfficeWorkflowProblem{{
				Code: OfficeProblemUnknownAgentProfile, StepName: "Review", Action: string(OnEnterRunCodeReview),
			}},
		},
		{
			name: "AC-005.6 several problems are all reported in step order",
			mutate: func(s []*WorkflowStep) []*WorkflowStep {
				stepByID(s, "work").IsStartStep = false
				stepByID(s, "done").CompleteTaskOnEnter = false
				stepByID(s, "review").Events.OnEnter = nil
				return s
			},
			want: []OfficeWorkflowProblem{
				{Code: OfficeProblemStartStepCount, Count: 0},
				{Code: OfficeProblemMissingSeatAction, StepName: "Review", Role: "reviewer", Action: string(OnEnterEnsureParticipantSeat)},
				{Code: OfficeProblemMissingSeatAction, StepName: "Review", Role: "reviewer", Action: string(OnEnterQueueRunForEachParticipant)},
				{Code: OfficeProblemFinalStepNotCompleting, StepName: "Done"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			exists := tc.exists
			if exists == nil {
				exists = allProfilesExist
			}
			got := ValidateOfficeWorkflow(tc.mutate(validOfficeSteps()), exists)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("problems =\n  %+v\nwant\n  %+v", got, tc.want)
			}
		})
	}
}

func TestValidateOfficeWorkflowFollowsMoveToNextAndGenericTriggers(t *testing.T) {
	steps := validOfficeSteps()
	stepByID(steps, "work").Events.OnTurnComplete = []OnTurnCompleteAction{{Type: OnTurnCompleteMoveToNext}}
	stepByID(steps, "review").Events.OnTurnComplete = nil
	stepByID(steps, "review").Events.OnComment = []GenericAction{{Type: GenericActionMoveToNext}}
	stepByID(steps, "approval").Events.OnTurnComplete = nil
	stepByID(steps, "approval").Events.OnApprovalResolved = []GenericAction{
		{Type: GenericActionMoveToStep, Config: map[string]interface{}{"step_id": "done"}},
	}
	if problems := ValidateOfficeWorkflow(steps, allProfilesExist); len(problems) != 0 {
		t.Fatalf("problems = %+v, want none", problems)
	}
}

func TestValidateOfficeWorkflowEmpty(t *testing.T) {
	got := ValidateOfficeWorkflow(nil, allProfilesExist)
	want := []OfficeWorkflowProblem{
		{Code: OfficeProblemStartStepCount, Count: 0},
		{Code: OfficeProblemMissingWorkStage},
		{Code: OfficeProblemFinalStepNotCompleting},
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("problems = %+v, want %+v", got, want)
	}
}
