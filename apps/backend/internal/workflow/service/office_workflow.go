package service

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"go.uber.org/zap"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

// OfficeWorkflowRefusal reports why an Office workflow write was refused. It
// carries every problem so the client can render one sentence per problem.
type OfficeWorkflowRefusal struct {
	Problems []models.OfficeWorkflowProblem
	// Conflict marks a refusal of the request's shape (a write the target
	// does not accept) rather than of the workflow's content.
	Conflict bool
}

func (e *OfficeWorkflowRefusal) Error() string {
	codes := make([]string, 0, len(e.Problems))
	for _, p := range e.Problems {
		codes = append(codes, p.Code)
	}
	return "office workflow refused: " + strings.Join(codes, ", ")
}

// AsOfficeWorkflowRefusal unwraps an OfficeWorkflowRefusal from err.
func AsOfficeWorkflowRefusal(err error) (*OfficeWorkflowRefusal, bool) {
	var refusal *OfficeWorkflowRefusal
	if errors.As(err, &refusal) {
		return refusal, true
	}
	var stepRefusal *repository.OfficeStepWriteRefusedError
	if errors.As(err, &stepRefusal) {
		return &OfficeWorkflowRefusal{Problems: []models.OfficeWorkflowProblem{{Code: stepRefusal.Code}}, Conflict: true}, true
	}
	return nil, false
}

func refuseOffice(conflict bool, problems ...models.OfficeWorkflowProblem) error {
	return &OfficeWorkflowRefusal{Problems: problems, Conflict: conflict}
}

// ErrWorkspaceManageDenied reports that the caller can see the workspace but
// may not manage its settings.
var ErrWorkspaceManageDenied = models.ErrWorkspaceManageDenied

// OfficeWorkflowCandidate is one workflow the workspace may select.
type OfficeWorkflowCandidate struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	BuiltIn  bool   `json:"built_in"`
	IsOffice bool   `json:"is_office"`
}

// OfficeWorkflowState is the workspace's selection and its candidates.
type OfficeWorkflowState struct {
	SelectedWorkflowID string                    `json:"selected_workflow_id"`
	Candidates         []OfficeWorkflowCandidate `json:"candidates"`
}

// SetWorkspaceManageChecker wires the task-domain workspace-manage scope
// check. Production passes a closure over AuthorizeWorkspaceScope with
// authz.ScopeWorkspaceManage.
func (s *Service) SetWorkspaceManageChecker(checker func(context.Context, string) error) {
	s.workspaceManageChecker = checker
}

// SetOfficeWorkflowChangePublisher wires the notification sent after an
// Office workflow selection or membership change commits.
func (s *Service) SetOfficeWorkflowChangePublisher(publish func(ctx context.Context, workspaceID, workflowID string)) {
	s.officeWorkflowChanged = publish
}

func (s *Service) publishOfficeWorkflowChanged(ctx context.Context, workspaceID, workflowID string) {
	if s.officeWorkflowChanged != nil {
		s.officeWorkflowChanged(ctx, workspaceID, workflowID)
	}
}

// OfficeWorkflowState lists the workspace's selected Office workflow and the
// workflows it may select instead.
func (s *Service) OfficeWorkflowState(ctx context.Context, workspaceID string) (*OfficeWorkflowState, error) {
	if err := s.AuthorizeWorkspace(ctx, workspaceID); err != nil {
		return nil, err
	}
	selected, rows, err := s.repo.ListOfficeWorkflowRows(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	state := &OfficeWorkflowState{SelectedWorkflowID: selected, Candidates: []OfficeWorkflowCandidate{}}
	for _, row := range rows {
		if officeEligibilityProblem(row) != nil {
			continue
		}
		state.Candidates = append(state.Candidates, OfficeWorkflowCandidate{
			ID: row.ID, Name: row.Name, BuiltIn: row.IsSystem, IsOffice: row.IsOffice,
		})
	}
	return state, nil
}

// SelectOfficeWorkflow makes workflowID the workspace's selected Office
// workflow after checking eligibility and Office validation against the
// locked rows. New Office tasks start on it; existing tasks keep theirs.
func (s *Service) SelectOfficeWorkflow(ctx context.Context, workspaceID, workflowID string) error {
	if err := s.authorizeWorkspaceManage(ctx, workspaceID); err != nil {
		return err
	}
	problems, err := s.repo.SelectOfficeWorkflow(ctx, workspaceID, workflowID,
		func(row *repository.OfficeWorkflowRow, steps []*models.WorkflowStep) []models.OfficeWorkflowProblem {
			if problem := officeEligibilityProblem(row); problem != nil {
				return []models.OfficeWorkflowProblem{*problem}
			}
			return models.ValidateOfficeWorkflow(steps, s.officeProfileExists)
		})
	if err != nil {
		return fmt.Errorf("select office workflow: %w", err)
	}
	if len(problems) > 0 {
		return refuseOffice(false, problems...)
	}
	s.logger.Info("office workflow selected",
		zap.String("workspace_id", workspaceID), zap.String("workflow_id", workflowID))
	s.publishOfficeWorkflowChanged(ctx, workspaceID, workflowID)
	return nil
}

func (s *Service) authorizeWorkspaceManage(ctx context.Context, workspaceID string) error {
	if err := s.AuthorizeWorkspace(ctx, workspaceID); err != nil {
		return err
	}
	if s.workspaceManageChecker == nil {
		return nil
	}
	return s.workspaceManageChecker(ctx, workspaceID)
}

// officeEligibilityProblem reports why a workflow row cannot be selected, or
// nil when it can: Office workflows that are not sync-managed, and visible
// user workflows that hold no tasks.
func officeEligibilityProblem(row *repository.OfficeWorkflowRow) *models.OfficeWorkflowProblem {
	switch {
	case row.Source != taskmodels.WorkflowSourceManual:
		return &models.OfficeWorkflowProblem{Code: models.OfficeProblemSyncManaged}
	case row.IsOffice:
		return nil
	case row.IsSystem || row.Hidden:
		return &models.OfficeWorkflowProblem{Code: models.OfficeProblemNotSelectable}
	case row.TaskCount > 0:
		return &models.OfficeWorkflowProblem{Code: models.OfficeProblemHoldsKanbanTasks}
	}
	return nil
}

// officeProfileExists reports whether an agent profile referenced by an
// Office action resolves in this installation.
func (s *Service) officeProfileExists(id string) bool {
	if s.resolveProfile == nil {
		return true
	}
	return s.resolveProfile(id) != nil
}

// SetOfficeStepWritePolicy wires the task-domain check that names an Office
// workflow refusing single-step writes: the built-in Office workflow and the
// workspace's selected Office workflow. It returns a problem code, or "" when
// the write may proceed.
func (s *Service) SetOfficeStepWritePolicy(policy func(ctx context.Context, workflowID string) (string, error)) {
	s.officeStepWritePolicy = policy
}

// withOfficeStepGuard marks a single-step write for the repository's
// in-transaction Office check when an Office step policy is wired.
func (s *Service) withOfficeStepGuard(ctx context.Context) context.Context {
	if s.officeStepWritePolicy == nil {
		return ctx
	}
	return repository.WithOfficeStepGuard(ctx)
}

func (s *Service) ensureOfficeStepWriteAllowed(ctx context.Context, workflowID string) error {
	if s.officeStepWritePolicy == nil {
		return nil
	}
	code, err := s.officeStepWritePolicy(ctx, workflowID)
	if err != nil {
		return err
	}
	if code != "" {
		return refuseOffice(true, models.OfficeWorkflowProblem{Code: code})
	}
	return nil
}
