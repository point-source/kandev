package service

import (
	"context"

	"github.com/kandev/kandev/internal/workflow/models"
)

// ensureOfficeImportAllowed refuses an import that carries an Office
// workflow into a workspace without Office, before anything is written.
func (s *Service) ensureOfficeImportAllowed(ctx context.Context, workspaceID string, workflows []models.WorkflowPortable) error {
	var officeName string
	for _, wf := range workflows {
		if wf.Office {
			officeName = wf.Name
			break
		}
	}
	if officeName == "" {
		return nil
	}
	selected, err := s.repo.WorkspaceOfficeWorkflowID(ctx, workspaceID)
	if err != nil {
		return err
	}
	if selected == "" {
		return refuseOffice(false, models.OfficeWorkflowProblem{
			Code: models.OfficeProblemRequiresOfficeWorkspace, Detail: officeName,
		})
	}
	return nil
}

// persistOfficeImport creates an imported Office workflow as an Office copy
// in one transaction.
func (s *Service) persistOfficeImport(ctx context.Context, workspaceID string, prepared preparedImportedWorkflow) error {
	pw := prepared.portable
	_, err := s.SaveOfficeWorkflowDefinition(ctx, OfficeWorkflowDefinition{
		WorkspaceID: workspaceID, Name: pw.Name, Description: pw.Description, Prompt: pw.Prompt,
		AgentProfileID: prepared.workflowProfileID, Steps: prepared.steps,
	})
	return err
}
