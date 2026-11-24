package models

import "fmt"

var knownOnEnterActions = map[OnEnterActionType]bool{
	OnEnterConfigureSession: true, OnEnterEnablePlanMode: true, OnEnterAutoStartAgent: true,
	OnEnterResetAgentContext: true, OnEnterSetSessionMode: true, OnEnterClearDecisions: true,
	OnEnterQueueRunForEachParticipant: true, OnEnterQueueRun: true, OnEnterEnsureParticipantSeat: true,
	OnEnterRunCodeReview: true,
}

var knownGenericActions = map[GenericActionType]bool{
	GenericActionMoveToNext: true, GenericActionMoveToPrevious: true, GenericActionMoveToStep: true,
	GenericActionAutoStartAgent: true, GenericActionQueueRun: true, GenericActionClearDecisions: true,
	GenericActionQueueRunForEachParticipant: true,
}

// ValidateKnownStepActions reports the first action whose type its trigger
// does not support. The engine silently ignores unknown actions, so writers
// that accept free-form events check them before saving.
func ValidateKnownStepActions(events StepEvents) error {
	for _, a := range events.OnEnter {
		if !knownOnEnterActions[a.Type] {
			return unknownAction("on_enter", string(a.Type))
		}
	}
	for _, a := range events.OnTurnStart {
		switch a.Type {
		case OnTurnStartMoveToNext, OnTurnStartMoveToPrevious, OnTurnStartMoveToStep:
		default:
			return unknownAction("on_turn_start", string(a.Type))
		}
	}
	for _, a := range events.OnTurnComplete {
		switch a.Type {
		case OnTurnCompleteMoveToNext, OnTurnCompleteMoveToPrevious, OnTurnCompleteMoveToStep, OnTurnCompleteDisablePlanMode:
		default:
			return unknownAction("on_turn_complete", string(a.Type))
		}
	}
	for _, a := range events.OnExit {
		if a.Type != OnExitDisablePlanMode {
			return unknownAction("on_exit", string(a.Type))
		}
	}
	for trigger, actions := range genericTriggersByName(events) {
		for _, a := range actions {
			if !knownGenericActions[a.Type] {
				return unknownAction(trigger, string(a.Type))
			}
		}
	}
	return nil
}

func unknownAction(trigger, action string) error {
	return fmt.Errorf("unknown action %q in %s", action, trigger)
}

func genericTriggersByName(events StepEvents) map[string][]GenericAction {
	return map[string][]GenericAction{
		"on_comment": events.OnComment, "on_blocker_resolved": events.OnBlockerResolved,
		"on_children_completed": events.OnChildrenCompleted, "on_approval_resolved": events.OnApprovalResolved,
		"on_heartbeat": events.OnHeartbeat, "on_budget_alert": events.OnBudgetAlert,
		"on_agent_error": events.OnAgentError, "on_pr_merged": events.OnPRMerged, "on_pr_closed": events.OnPRClosed,
	}
}
