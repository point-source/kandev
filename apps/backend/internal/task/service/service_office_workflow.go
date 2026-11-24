package service

import (
	"context"
	"errors"
	"fmt"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// AuthorizeWorkspaceManage reports whether the caller may manage the
// workspace's settings, such as its selected Office workflow. A visible
// workspace without the scope also matches wfmodels.ErrWorkspaceManageDenied
// so the workflow surface can answer 403 without importing this package.
func (s *Service) AuthorizeWorkspaceManage(ctx context.Context, workspaceID string) error {
	err := s.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage)
	if IsForbidden(err) {
		return fmt.Errorf("%w: %w", wfmodels.ErrWorkspaceManageDenied, err)
	}
	return err
}

// PublishOfficeWorkflowChanged tells clients that a workspace's Office
// workflow selection or a workflow's Office membership changed, so Kanban
// listings, the boot store and settings refresh.
func (s *Service) PublishOfficeWorkflowChanged(ctx context.Context, workspaceID, workflowID string) {
	if workspace, err := s.workspaces.GetWorkspace(ctx, workspaceID); err == nil {
		s.publishWorkspaceEvent(ctx, events.WorkspaceUpdated, workspace)
	} else {
		s.logger.Warn("office workflow change: load workspace failed", zap.String("workspace_id", workspaceID), zap.Error(err))
	}
	if workflowID == "" {
		return
	}
	if workflow, err := s.workflows.GetWorkflow(ctx, workflowID); err == nil {
		s.publishWorkflowEvent(ctx, events.WorkflowUpdated, workflow)
	} else {
		s.logger.Warn("office workflow change: load workflow failed", zap.String("workflow_id", workflowID), zap.Error(err))
	}
}

// OfficeWorkflowDeleteRefusal reports that a workflow cannot be deleted
// because Office depends on it. Code is an Office workflow problem code.
type OfficeWorkflowDeleteRefusal struct {
	Code string
}

func (e *OfficeWorkflowDeleteRefusal) Error() string {
	return "office workflow cannot be deleted: " + e.Code
}

// checkOfficeWorkflowDeletable refuses deleting the built-in Office workflow
// and the workspace's selected Office workflow.
func (s *Service) checkOfficeWorkflowDeletable(ctx context.Context, workflow *models.Workflow) error {
	if workflow.IsOffice && workflow.IsSystem {
		return &OfficeWorkflowDeleteRefusal{Code: wfmodels.OfficeProblemBuiltInReadOnly}
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workflow.WorkspaceID)
	if err != nil {
		return err
	}
	if workspace != nil && workspace.OfficeWorkflowID == workflow.ID {
		return &OfficeWorkflowDeleteRefusal{Code: wfmodels.OfficeProblemSelectedDelete}
	}
	return nil
}

// OfficeStepWriteRefusal names why a single-step write to workflowID is
// refused, or returns "" when it may proceed. The built-in Office workflow
// is read-only, and the selected Office workflow changes only through a
// whole-definition save so it never passes through a shape Office cannot
// drive.
func (s *Service) OfficeStepWriteRefusal(ctx context.Context, workflowID string) (string, error) {
	workflow, err := s.workflows.GetWorkflow(ctx, workflowID)
	if errors.Is(err, repoerrors.ErrWorkflowNotFound) || (err == nil && workflow == nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if workflow.IsOffice && workflow.IsSystem {
		return wfmodels.OfficeProblemBuiltInReadOnly, nil
	}
	workspace, err := s.workspaces.GetWorkspace(ctx, workflow.WorkspaceID)
	if errors.Is(err, repoerrors.ErrWorkspaceNotFound) || (err == nil && workspace == nil) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if workspace.OfficeWorkflowID == workflowID {
		return wfmodels.OfficeProblemSelectedRequiresDefinitionSave, nil
	}
	return "", nil
}
