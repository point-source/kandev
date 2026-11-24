package models

import "testing"

func TestValidateKnownStepActions(t *testing.T) {
	if err := ValidateKnownStepActions(validOfficeSteps()[2].Events); err != nil {
		t.Fatalf("Office review events rejected: %v", err)
	}
	cases := map[string]StepEvents{
		"on_enter":         {OnEnter: []OnEnterAction{{Type: "ensure_seat"}}},
		"on_turn_start":    {OnTurnStart: []OnTurnStartAction{{Type: "queue_run"}}},
		"on_turn_complete": {OnTurnComplete: []OnTurnCompleteAction{{Type: "run_code_review"}}},
		"on_exit":          {OnExit: []OnExitAction{{Type: "move_to_next"}}},
		"on_comment":       {OnComment: []GenericAction{{Type: "run_code_review"}}},
	}
	for name, events := range cases {
		if err := ValidateKnownStepActions(events); err == nil {
			t.Errorf("%s: unknown action accepted", name)
		}
	}
}
