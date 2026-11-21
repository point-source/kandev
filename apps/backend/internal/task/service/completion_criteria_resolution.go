package service

import (
	"context"
	"errors"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

// CompletionCriteriaRefusal refuses a task creation because of its completion
// criteria. Code is a stable client-facing code.
type CompletionCriteriaRefusal struct {
	Code string
}

func (e *CompletionCriteriaRefusal) Error() string { return e.Code }

// ErrCompletionCriteriaRequireOfficeTask refuses criteria on a non-Office task.
var ErrCompletionCriteriaRequireOfficeTask error = &CompletionCriteriaRefusal{Code: "completion_criteria_requires_office_task"}

// IsCompletionCriteriaRefusal reports a creation refused for its criteria and
// returns the code.
func IsCompletionCriteriaRefusal(err error) (string, bool) {
	var refusal *CompletionCriteriaRefusal
	if errors.As(err, &refusal) {
		return refusal.Code, true
	}
	return "", false
}

// CompletionCriteriaResolver turns a creator's request and the workspace
// policy into the criteria stored with a new task.
type CompletionCriteriaResolver interface {
	ResolveCreateCriteria(ctx context.Context, input models.CompletionCriteriaResolution) ([]models.TaskCompletionCriterion, error)
}

// CompletionCriteriaChangeValidator refuses an edited criterion set the owning
// system cannot verify. A resolver may also implement it.
type CompletionCriteriaChangeValidator interface {
	ValidateCriteriaChange(ctx context.Context, task *models.Task, criteria []models.TaskCompletionCriterion) error
}

// validateCompletionCriteriaChange applies the registered policy to an edit.
func (s *Service) validateCompletionCriteriaChange(ctx context.Context, task *models.Task, criteria []models.TaskCompletionCriterion) error {
	validator, ok := s.completionCriteriaResolver.(CompletionCriteriaChangeValidator)
	if !ok || task == nil {
		return nil
	}
	return validator.ValidateCriteriaChange(ctx, task, criteria)
}

// SetCompletionCriteriaResolver registers the creation-criteria policy.
func (s *Service) SetCompletionCriteriaResolver(resolver CompletionCriteriaResolver) {
	s.completionCriteriaResolver = resolver
}

// resolveCompletionCriteria fills req.CompletionCriteria for an Office task
// through the registered resolver. Criteria named for a non-Office task are
// refused.
func (s *Service) resolveCompletionCriteria(ctx context.Context, req *CreateTaskRequest) error {
	office := isOfficeRequest(req)
	if !office {
		if req.CompletionCriteriaRequest != nil && len(*req.CompletionCriteriaRequest) > 0 {
			return ErrCompletionCriteriaRequireOfficeTask
		}
		return nil
	}
	if s.completionCriteriaResolver == nil || len(req.CompletionCriteria) > 0 {
		return nil
	}
	criteria, err := s.completionCriteriaResolver.ResolveCreateCriteria(ctx, models.CompletionCriteriaResolution{
		WorkspaceID: req.WorkspaceID, WorkflowID: req.WorkflowID, IsOffice: office,
		HasRepository: len(req.Repositories) > 0, Requested: req.CompletionCriteriaRequest,
		Creator: completionCriteriaCreator(ctx, req),
	})
	if err != nil {
		return err
	}
	req.CompletionCriteria = criteria
	return nil
}

// completionCriteriaCreator uses the explicit creator, then the request's
// origin, then the authenticated person.
func completionCriteriaCreator(ctx context.Context, req *CreateTaskRequest) models.CompletionCriteriaCreator {
	if req.CompletionCriteriaCreator.Kind != "" {
		return req.CompletionCriteriaCreator
	}
	switch req.Origin {
	case models.TaskOriginAgentCreated:
		return models.CompletionCriteriaCreator{Kind: models.CompletionCriteriaCreatorAgent}
	case models.TaskOriginRoutine:
		return models.CompletionCriteriaCreator{Kind: models.CompletionCriteriaCreatorRoutine}
	}
	if identity, ok := authn.IdentityFromContext(ctx); ok && strings.TrimSpace(identity.UserID) != "" {
		return models.CompletionCriteriaCreator{Kind: models.CompletionCriteriaCreatorHuman, ID: identity.UserID}
	}
	return models.CompletionCriteriaCreator{Kind: models.CompletionCriteriaCreatorRoutine}
}
