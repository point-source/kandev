package models

import "sort"

// Office workflow problem codes. Each names one rule a workflow must satisfy
// before Office can drive it (REQ-OFFICE-WORKFLOW-SELECT-005). The web client
// translates each code into a localized problem-and-fix sentence.
const (
	OfficeProblemStartStepCount            = "start_step_count"
	OfficeProblemMissingWorkStage          = "missing_work_stage"
	OfficeProblemFinalStepNotCompleting    = "final_step_not_completing"
	OfficeProblemCompletingStepUnreachable = "completing_step_unreachable"
	OfficeProblemMissingSeatAction         = "missing_seat_action"
	OfficeProblemUnknownAgentProfile       = "unknown_agent_profile"
)

// Office workflow refusal codes that are not structural validation rules.
const (
	OfficeProblemNotInWorkspace                 = "not_in_workspace"
	OfficeProblemNotSelectable                  = "not_selectable"
	OfficeProblemSyncManaged                    = "sync_managed"
	OfficeProblemHoldsKanbanTasks               = "holds_kanban_tasks"
	OfficeProblemRequiresOfficeWorkspace        = "requires_office_workspace"
	OfficeProblemBuiltInReadOnly                = "built_in_read_only"
	OfficeProblemSelectedDelete                 = "selected_delete"
	OfficeProblemStepHoldsTasks                 = "step_holds_tasks"
	OfficeProblemSelectedRequiresDefinitionSave = "selected_requires_definition_save"
	OfficeProblemInvalidEvents                  = "invalid_events"
	OfficeProblemNotOfficeWorkflow              = "not_office_workflow"
)

// OfficeWorkflowProblem is one reason a workflow cannot be an Office
// workflow. Fields other than Code are set only when the rule names them.
type OfficeWorkflowProblem struct {
	Code     string `json:"code"`
	StepName string `json:"step_name,omitempty"`
	Role     string `json:"role,omitempty"`
	Action   string `json:"action,omitempty"`
	Count    int    `json:"count,omitempty"`
	Detail   string `json:"detail,omitempty"`
}

const (
	moveTargetConfigKey = "step_id"
	seatRoleConfigKey   = "role"
)

// ValidateOfficeWorkflow returns every rule the steps fail. Workflow-wide
// problems come first, then step problems in step-position order. An empty
// result means Office can drive the workflow.
func ValidateOfficeWorkflow(steps []*WorkflowStep, profileExists func(id string) bool) []OfficeWorkflowProblem {
	ordered := sortedByPosition(steps)
	problems := workflowWideOfficeProblems(ordered)
	for i, step := range ordered {
		problems = append(problems, seatProblems(step)...)
		problems = append(problems, unknownProfileProblems(step, profileExists)...)
		if i == len(ordered)-1 {
			problems = append(problems, completingStepProblems(ordered)...)
		}
	}
	if len(ordered) == 0 {
		problems = append(problems, OfficeWorkflowProblem{Code: OfficeProblemFinalStepNotCompleting})
	}
	return problems
}

func sortedByPosition(steps []*WorkflowStep) []*WorkflowStep {
	ordered := make([]*WorkflowStep, 0, len(steps))
	for _, step := range steps {
		if step != nil {
			ordered = append(ordered, step)
		}
	}
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Position < ordered[j].Position })
	return ordered
}

func workflowWideOfficeProblems(steps []*WorkflowStep) []OfficeWorkflowProblem {
	var problems []OfficeWorkflowProblem
	starts, hasWork := 0, false
	for _, step := range steps {
		if step.IsStartStep {
			starts++
		}
		if step.StageType == StageTypeWork && step.AutoAdvanceRequiresSignal {
			hasWork = true
		}
	}
	if starts != 1 {
		problems = append(problems, OfficeWorkflowProblem{Code: OfficeProblemStartStepCount, Count: starts})
	}
	if !hasWork {
		problems = append(problems, OfficeWorkflowProblem{Code: OfficeProblemMissingWorkStage})
	}
	return problems
}

func seatProblems(step *WorkflowStep) []OfficeWorkflowProblem {
	role := ""
	switch step.StageType {
	case StageTypeReview:
		role = "reviewer"
	case StageTypeApproval:
		role = "approver"
	default:
		return nil
	}
	var problems []OfficeWorkflowProblem
	for _, required := range []OnEnterActionType{OnEnterEnsureParticipantSeat, OnEnterQueueRunForEachParticipant} {
		if !hasOnEnterRoleAction(step, required, role) {
			problems = append(problems, OfficeWorkflowProblem{
				Code: OfficeProblemMissingSeatAction, StepName: step.Name, Role: role, Action: string(required),
			})
		}
	}
	return problems
}

func hasOnEnterRoleAction(step *WorkflowStep, actionType OnEnterActionType, role string) bool {
	for _, action := range step.Events.OnEnter {
		if action.Type != actionType {
			continue
		}
		if got, _ := action.Config[seatRoleConfigKey].(string); got == role {
			return true
		}
	}
	return false
}

func unknownProfileProblems(step *WorkflowStep, profileExists func(string) bool) []OfficeWorkflowProblem {
	var problems []OfficeWorkflowProblem
	check := func(actionType string, config map[string]interface{}) {
		id, _ := config[ReviewAgentProfileConfigKey].(string)
		if id != "" && (profileExists == nil || !profileExists(id)) {
			problems = append(problems, OfficeWorkflowProblem{
				Code: OfficeProblemUnknownAgentProfile, StepName: step.Name, Action: actionType,
			})
		}
	}
	for _, action := range step.Events.OnEnter {
		check(string(action.Type), action.Config)
	}
	for _, actions := range genericTriggerActions(step.Events) {
		for _, action := range actions {
			check(string(action.Type), action.Config)
		}
	}
	return problems
}

func completingStepProblems(ordered []*WorkflowStep) []OfficeWorkflowProblem {
	final := ordered[len(ordered)-1]
	if !final.CompleteTaskOnEnter {
		return []OfficeWorkflowProblem{{Code: OfficeProblemFinalStepNotCompleting, StepName: final.Name}}
	}
	// Reachability is only defined from a single start step; a wrong start
	// count is already reported as its own problem.
	var starts []*WorkflowStep
	for _, step := range ordered {
		if step.IsStartStep {
			starts = append(starts, step)
		}
	}
	if len(starts) != 1 {
		return nil
	}
	if !reachable(ordered, starts[0], final) {
		return []OfficeWorkflowProblem{{Code: OfficeProblemCompletingStepUnreachable, StepName: final.Name}}
	}
	return nil
}

// reachable walks the step-move actions of every trigger from start.
func reachable(ordered []*WorkflowStep, start, target *WorkflowStep) bool {
	index := make(map[string]int, len(ordered))
	for i, step := range ordered {
		index[step.ID] = i
	}
	seen := map[int]bool{index[start.ID]: true}
	queue := []int{index[start.ID]}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if ordered[current].ID == target.ID {
			return true
		}
		for _, next := range moveTargets(ordered, current, index) {
			if !seen[next] {
				seen[next] = true
				queue = append(queue, next)
			}
		}
	}
	return false
}

func moveTargets(ordered []*WorkflowStep, current int, index map[string]int) []int {
	var targets []int
	add := func(actionType string, config map[string]interface{}) {
		switch actionType {
		case string(GenericActionMoveToNext):
			if current+1 < len(ordered) {
				targets = append(targets, current+1)
			}
		case string(GenericActionMoveToPrevious):
			if current > 0 {
				targets = append(targets, current-1)
			}
		case string(GenericActionMoveToStep):
			id, _ := config[moveTargetConfigKey].(string)
			if i, ok := index[id]; ok {
				targets = append(targets, i)
			}
		}
	}
	events := ordered[current].Events
	for _, action := range events.OnTurnStart {
		add(string(action.Type), action.Config)
	}
	for _, action := range events.OnTurnComplete {
		add(string(action.Type), action.Config)
	}
	for _, actions := range genericTriggerActions(events) {
		for _, action := range actions {
			add(string(action.Type), action.Config)
		}
	}
	return targets
}

func genericTriggerActions(events StepEvents) [][]GenericAction {
	return [][]GenericAction{
		events.OnComment, events.OnBlockerResolved, events.OnChildrenCompleted,
		events.OnApprovalResolved, events.OnHeartbeat, events.OnBudgetAlert,
		events.OnAgentError, events.OnPRMerged, events.OnPRClosed,
	}
}
