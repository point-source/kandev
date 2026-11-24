package service

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

// OfficeWorkflowDefinition is a complete Office copy: workflow metadata and
// every step. Steps whose ID names an existing step of the workflow are
// updated; any other ID is a client key for a new step, and step references
// that use the key are rewritten to the new step's ID.
type OfficeWorkflowDefinition struct {
	// WorkspaceID creates a new Office copy in this workspace when
	// WorkflowID is empty.
	WorkspaceID       string                 `json:"workspace_id,omitempty"`
	WorkflowID        string                 `json:"-"`
	ExpectedUpdatedAt *time.Time             `json:"expected_updated_at,omitempty"`
	Name              string                 `json:"name"`
	Description       string                 `json:"description"`
	Prompt            string                 `json:"prompt"`
	AgentProfileID    string                 `json:"agent_profile_id"`
	Steps             []*models.WorkflowStep `json:"steps"`
}

// OfficeWorkflowSaveResult reports a committed Office definition save.
type OfficeWorkflowSaveResult struct {
	WorkflowID  string                 `json:"workflow_id"`
	WorkspaceID string                 `json:"workspace_id"`
	UpdatedAt   time.Time              `json:"updated_at"`
	Steps       []*models.WorkflowStep `json:"steps"`
	Created     []*models.WorkflowStep `json:"-"`
	Updated     []*models.WorkflowStep `json:"-"`
	Deleted     []*models.WorkflowStep `json:"-"`
}

// officeTargetConflicts are refusals of the write target itself; they answer
// 409 rather than 422.
var officeTargetConflicts = map[string]bool{
	models.OfficeProblemBuiltInReadOnly:   true,
	models.OfficeProblemNotOfficeWorkflow: true,
	models.OfficeProblemSyncManaged:       true,
}

// SaveOfficeWorkflowDefinition creates or replaces an Office copy and all of
// its steps in one transaction. When the target is the workspace's selected
// Office workflow, the resulting steps must pass Office validation.
func (s *Service) SaveOfficeWorkflowDefinition(ctx context.Context, def OfficeWorkflowDefinition) (*OfficeWorkflowSaveResult, error) {
	create := def.WorkflowID == ""
	workspaceID, err := s.authorizeOfficeDefinition(ctx, def)
	if err != nil {
		return nil, err
	}
	workflowID := def.WorkflowID
	if create {
		workflowID = uuid.New().String()
	}
	steps, err := s.prepareOfficeSteps(ctx, workflowID, def.Steps)
	if err != nil {
		return nil, err
	}
	result, problems, err := s.repo.SaveOfficeWorkflowDefinition(ctx, repository.OfficeDefinitionWrite{
		WorkspaceID: workspaceID, WorkflowID: workflowID, Create: create, CreatedByUserID: officeWorkflowCreator(ctx),
		ExpectedUpdatedAt: def.ExpectedUpdatedAt, Name: strings.TrimSpace(def.Name), Description: def.Description,
		Prompt: def.Prompt, AgentProfileID: def.AgentProfileID, Steps: steps,
	}, func(row *repository.OfficeWorkflowRow, selected bool, _ []*models.WorkflowStep) []models.OfficeWorkflowProblem {
		if problem := officeDefinitionTargetProblem(row); problem != nil {
			return []models.OfficeWorkflowProblem{*problem}
		}
		if selected {
			return models.ValidateOfficeWorkflow(steps, s.officeProfileExists)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(problems) > 0 {
		return nil, refuseOffice(officeTargetConflicts[problems[0].Code], problems...)
	}
	s.logger.Info("office workflow definition saved", zap.String("workflow_id", workflowID),
		zap.Int("created", len(result.Created)), zap.Int("updated", len(result.Updated)), zap.Int("deleted", len(result.Deleted)))
	s.publishOfficeWorkflowChanged(ctx, workspaceID, workflowID)
	return &OfficeWorkflowSaveResult{
		WorkflowID: workflowID, WorkspaceID: workspaceID, UpdatedAt: result.UpdatedAt, Steps: steps,
		Created: result.Created, Updated: result.Updated, Deleted: result.Deleted,
	}, nil
}

// authorizeOfficeDefinition checks the caller may write the target and
// returns the owning workspace.
func (s *Service) authorizeOfficeDefinition(ctx context.Context, def OfficeWorkflowDefinition) (string, error) {
	if strings.TrimSpace(def.Name) == "" {
		return "", refuseOffice(false, models.OfficeWorkflowProblem{Code: models.OfficeProblemInvalidEvents, Detail: "workflow name is required"})
	}
	if def.WorkflowID == "" {
		if err := s.AuthorizeWorkspace(ctx, def.WorkspaceID); err != nil {
			return "", err
		}
		return def.WorkspaceID, s.EnsureWorkspaceMutable(ctx, def.WorkspaceID)
	}
	if err := s.AuthorizeWorkflow(ctx, def.WorkflowID); err != nil {
		return "", err
	}
	if err := s.authorizeWorkflowWrite(ctx, def.WorkflowID); err != nil {
		return "", err
	}
	workspaceID, err := s.repo.WorkflowWorkspaceID(ctx, def.WorkflowID)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", repoerrors.ErrWorkflowNotFound, def.WorkflowID)
	}
	return workspaceID, err
}

func officeDefinitionTargetProblem(row *repository.OfficeWorkflowRow) *models.OfficeWorkflowProblem {
	switch {
	case row.IsSystem:
		return &models.OfficeWorkflowProblem{Code: models.OfficeProblemBuiltInReadOnly}
	case !row.IsOffice:
		return &models.OfficeWorkflowProblem{Code: models.OfficeProblemNotOfficeWorkflow}
	case row.Source != taskmodels.WorkflowSourceManual:
		return &models.OfficeWorkflowProblem{Code: models.OfficeProblemSyncManaged}
	}
	return nil
}

// prepareOfficeSteps validates the submitted steps and gives new steps their
// IDs. Existing IDs are kept: the repository updates any ID the workflow
// already owns and inserts the rest.
func (s *Service) prepareOfficeSteps(ctx context.Context, workflowID string, submitted []*models.WorkflowStep) ([]*models.WorkflowStep, error) {
	existing := map[string]bool{}
	if stored, err := s.repo.ListStepsByWorkflow(ctx, workflowID); err == nil {
		for _, step := range stored {
			existing[step.ID] = true
		}
	} else {
		return nil, err
	}
	idMap := make(map[string]string, len(submitted))
	names := make(map[string]bool, len(submitted))
	for _, step := range submitted {
		if step == nil {
			return nil, invalidOfficeEvents("", "step is required")
		}
		if names[step.Name] {
			return nil, invalidOfficeEvents(step.Name, "step names must be unique")
		}
		names[step.Name] = true
		if step.ID == "" || !existing[step.ID] {
			newID := uuid.New().String()
			if step.ID != "" {
				idMap[step.ID] = newID
			}
			step.ID = newID
		}
	}
	steps := make([]*models.WorkflowStep, 0, len(submitted))
	for _, original := range submitted {
		step := *original
		step.WorkflowID = workflowID
		step.Events = models.RemapStepEvents(step.Events, idMap)
		step.PullFromStepID = remapID(step.PullFromStepID, idMap)
		if step.SessionTarget != nil {
			target := *step.SessionTarget
			target.StepID = remapID(target.StepID, idMap)
			step.SessionTarget = &target
		}
		steps = append(steps, &step)
	}
	return steps, s.validateOfficeSteps(ctx, steps)
}

func (s *Service) validateOfficeSteps(ctx context.Context, steps []*models.WorkflowStep) error {
	ids := make(map[string]bool, len(steps))
	for _, step := range steps {
		ids[step.ID] = true
	}
	for _, step := range steps {
		if err := models.ValidateWorkflowStep(step); err != nil {
			return invalidOfficeEvents(step.Name, err.Error())
		}
		if err := models.ValidateKnownStepActions(step.Events); err != nil {
			return invalidOfficeEvents(step.Name, err.Error())
		}
		refs := models.CollectStepEventReferences(step.Events)
		for _, target := range refs.StepIDs {
			if !ids[target] {
				return invalidOfficeEvents(step.Name, fmt.Sprintf("step reference %q is not a step of this workflow", target))
			}
		}
		if step.PullFromStepID != "" && !ids[step.PullFromStepID] {
			return invalidOfficeEvents(step.Name, "pull_from_step_id is not a step of this workflow")
		}
		if step.SessionTarget != nil && step.SessionTarget.StepID != "" && !ids[step.SessionTarget.StepID] {
			return invalidOfficeEvents(step.Name, "session_target step is not a step of this workflow")
		}
		for _, taskID := range refs.TaskIDs {
			if err := s.AuthorizeTask(ctx, taskID); err != nil {
				return err
			}
		}
	}
	return nil
}

func invalidOfficeEvents(stepName, detail string) error {
	return refuseOffice(false, models.OfficeWorkflowProblem{
		Code: models.OfficeProblemInvalidEvents, StepName: stepName, Detail: detail,
	})
}

func remapID(id string, idMap map[string]string) string {
	if mapped, ok := idMap[id]; ok {
		return mapped
	}
	return id
}

// officeWorkflowCreator mirrors the task repository's creator attribution for
// workflows created outside the task service.
func officeWorkflowCreator(ctx context.Context) string {
	if actor, bound := taskmodels.ExecutorActorFromContext(ctx); bound {
		return actor
	}
	if identity, ok := authn.IdentityFromContext(ctx); ok && !identity.Synthetic {
		return identity.UserID
	}
	return ""
}
