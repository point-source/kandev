package routines

import (
	"context"
	"fmt"
	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// RoutineTaskOptions are persisted by the task creator before a routine task
// can start. BlockedBy uses the normal dependency and document-handoff relation.
type RoutineTaskOptions struct {
	ProjectID            string
	BlockedBy            []string
	RequiredSubtaskCount int
	// CompletionCriteria is nil when the template names none.
	CompletionCriteria *[]taskmodels.TaskCompletionCriterionRequest
}

type routineTaskCreatorWithContext interface {
	CreateOfficeTaskInWorkflowWithContext(
		context.Context, string, string, string, string, string, string, RoutineTaskOptions,
	) (string, error)
}

func (s *RoutineService) createRoutineTaskWithContext(
	ctx context.Context, routine *Routine, workflowID, title, description string, template taskTemplate,
) (string, error) {
	if template.ProjectID != "" || len(template.BlockedBy) > 0 || template.CompletionCriteria != nil {
		creator, ok := s.taskCreator.(routineTaskCreatorWithContext)
		if !ok {
			return "", fmt.Errorf("routine task creator does not support declared task context")
		}
		return creator.CreateOfficeTaskInWorkflowWithContext(
			ctx, routine.WorkspaceID, routine.AssigneeAgentProfileID, workflowID, title, description, routine.ID,
			RoutineTaskOptions{
				ProjectID:            template.ProjectID,
				BlockedBy:            append([]string(nil), template.BlockedBy...),
				RequiredSubtaskCount: template.RequiredSubtaskCount,
				CompletionCriteria:   template.CompletionCriteria,
			},
		)
	}
	if template.RequiredSubtaskCount > 0 {
		creator, ok := s.taskCreator.(routineTaskCreatorWithRequirements)
		if !ok {
			return "", fmt.Errorf("routine task creator does not support required subtask assignments")
		}
		return creator.CreateOfficeTaskInWorkflowWithRequirements(
			ctx, routine.WorkspaceID, "", routine.AssigneeAgentProfileID, workflowID,
			title, description, routine.ID, template.RequiredSubtaskCount,
		)
	}
	return s.taskCreator.CreateOfficeTaskInWorkflow(
		ctx, routine.WorkspaceID, "", routine.AssigneeAgentProfileID, workflowID, title, description, routine.ID,
	)
}
