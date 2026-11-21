package completiongates

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

// Creation refusals.
var (
	ErrCriteriaRequireRepository  error = &taskservice.CompletionCriteriaRefusal{Code: "completion_criteria_requires_repository"}
	ErrCriteriaRequireReviewStage error = &taskservice.CompletionCriteriaRefusal{Code: "completion_criteria_requires_review_stage"}
)

// creationRefusal maps validation errors to client-facing creation refusals.
func creationRefusal(err error) error {
	switch {
	case errors.Is(err, ErrLimitAboveCeiling):
		return &taskservice.CompletionCriteriaRefusal{Code: "completion_limit_above_ceiling"}
	case errors.Is(err, ErrInvalidProtectedPath):
		return &taskservice.CompletionCriteriaRefusal{Code: "invalid_protected_path"}
	case errors.Is(err, ErrInvalidCriterion):
		return &taskservice.CompletionCriteriaRefusal{Code: "invalid_completion_criterion"}
	default:
		return err
	}
}

// ReviewStageReader reports whether a workflow has a review stage.
type ReviewStageReader interface {
	HasReviewStage(ctx context.Context, workflowID string) (bool, error)
}

// ResolveCreateCriteria implements the task service's creation resolver:
// requested criteria, the workspace default, ownership, and validation.
func (s *Service) ResolveCreateCriteria(ctx context.Context, input models.CompletionCriteriaResolution) ([]models.TaskCompletionCriterion, error) {
	settings, err := s.effectiveSettings(ctx, input.WorkspaceID)
	if err != nil {
		return nil, err
	}
	requested, defaults := requestedSpecs(input.Requested), settings.DefaultCriteria
	if hasDuplicateKind(requested) {
		return nil, creationRefusal(ErrInvalidCriterion)
	}
	person := input.Creator.Kind == models.CompletionCriteriaCreatorHuman
	var specs []officemodels.CompletionGateCriterionSpec
	switch {
	case input.Requested == nil:
		specs = defaults
	case person:
		specs, defaults = requested, nil
	default:
		specs = unionSpecs(defaults, requested)
	}
	criteria := make([]models.TaskCompletionCriterion, 0, len(specs))
	for _, spec := range specs {
		isDefault := !containsKind(requested, spec.Kind)
		criterion, err := s.resolveSpec(ctx, input, settings, spec, person)
		if err != nil && isDefault && containsKind(defaults, spec.Kind) && isApplicabilityError(err) {
			s.logger.Info("workspace default completion criterion skipped",
				zap.String("workspace_id", input.WorkspaceID), zap.String("kind", spec.Kind), zap.Error(err))
			continue
		}
		if err != nil {
			return nil, creationRefusal(err)
		}
		criteria = append(criteria, criterion)
	}
	return criteria, nil
}

func (s *Service) resolveSpec(
	ctx context.Context, input models.CompletionCriteriaResolution, settings officemodels.CompletionGateSettings,
	spec officemodels.CompletionGateCriterionSpec, person bool,
) (models.TaskCompletionCriterion, error) {
	spec.ProtectedPaths = trimPaths(spec.ProtectedPaths)
	if err := ValidateCriterionSpec(spec); err != nil {
		return models.TaskCompletionCriterion{}, err
	}
	if !input.HasRepository {
		return models.TaskCompletionCriterion{}, ErrCriteriaRequireRepository
	}
	if spec.Kind == models.TaskCompletionCriterionReview {
		if err := s.requireReviewStage(ctx, input.WorkflowID); err != nil {
			return models.TaskCompletionCriterion{}, err
		}
	}
	above := spec.VerificationLimit > settings.VerificationLimitCeiling || spec.ReworkLimit > settings.ReworkLimitCeiling
	criterion := models.TaskCompletionCriterion{
		ID: models.TaskCompletionCriterionID(spec.Kind, spec.Runner), Kind: spec.Kind,
		Config: models.TaskCompletionCriterionConfig{
			Runner: spec.Runner, ProtectedPaths: spec.ProtectedPaths, VerificationLimit: spec.VerificationLimit,
			ReworkLimit: spec.ReworkLimit, ReviewWaitLimit: spec.ReviewWaitLimit,
		},
		OwnerKind: input.Creator.Kind, OwnerID: input.Creator.ID,
	}
	if above && !person {
		return models.TaskCompletionCriterion{}, ErrLimitAboveCeiling
	}
	if above {
		criterion.Config.LimitsConfirmedBy = input.Creator.ID
	}
	return criterion, nil
}

func (s *Service) requireReviewStage(ctx context.Context, workflowID string) error {
	if s.stages == nil {
		return nil
	}
	ok, err := s.stages.HasReviewStage(ctx, workflowID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrCriteriaRequireReviewStage
	}
	return nil
}

func isApplicabilityError(err error) bool {
	return errors.Is(err, ErrCriteriaRequireRepository) || errors.Is(err, ErrCriteriaRequireReviewStage)
}

func hasDuplicateKind(specs []officemodels.CompletionGateCriterionSpec) bool {
	seen := map[string]bool{}
	for _, spec := range specs {
		if seen[spec.Kind] {
			return true
		}
		seen[spec.Kind] = true
	}
	return false
}

func requestedSpecs(requested *[]models.TaskCompletionCriterionRequest) []officemodels.CompletionGateCriterionSpec {
	if requested == nil {
		return nil
	}
	specs := make([]officemodels.CompletionGateCriterionSpec, 0, len(*requested))
	for _, request := range *requested {
		specs = append(specs, officemodels.CompletionGateCriterionSpec{
			Kind: strings.TrimSpace(request.Kind), Runner: strings.TrimSpace(request.Runner),
			ProtectedPaths: request.ProtectedPaths, VerificationLimit: request.VerificationLimit,
			ReworkLimit: request.ReworkLimit, ReviewWaitLimit: request.ReviewWaitLimit,
		})
	}
	return specs
}

// unionSpecs keeps every default and adds requested kinds; a requested spec
// replaces a default of the same kind only by adding protected paths and
// limits, never by dropping them.
func unionSpecs(defaults, requested []officemodels.CompletionGateCriterionSpec) []officemodels.CompletionGateCriterionSpec {
	merged := append([]officemodels.CompletionGateCriterionSpec(nil), defaults...)
	for _, spec := range requested {
		index := indexOfKind(merged, spec.Kind)
		if index < 0 {
			merged = append(merged, spec)
			continue
		}
		merged[index].ProtectedPaths = append(merged[index].ProtectedPaths, spec.ProtectedPaths...)
		if spec.VerificationLimit > 0 {
			merged[index].VerificationLimit = spec.VerificationLimit
		}
		if spec.ReworkLimit > 0 {
			merged[index].ReworkLimit = spec.ReworkLimit
		}
		if spec.ReviewWaitLimit > 0 {
			merged[index].ReviewWaitLimit = spec.ReviewWaitLimit
		}
	}
	return merged
}

func indexOfKind(specs []officemodels.CompletionGateCriterionSpec, kind string) int {
	for index, spec := range specs {
		if spec.Kind == kind {
			return index
		}
	}
	return -1
}

func containsKind(specs []officemodels.CompletionGateCriterionSpec, kind string) bool {
	return indexOfKind(specs, kind) >= 0
}

func trimPaths(paths []string) []string {
	trimmed := make([]string, 0, len(paths))
	for _, path := range paths {
		trimmed = append(trimmed, strings.TrimSpace(path))
	}
	return trimmed
}

// ValidateCriteriaChange refuses an edited criterion set with a review when
// the task's workflow has no review stage, as at creation.
func (s *Service) ValidateCriteriaChange(ctx context.Context, task *models.Task, criteria []models.TaskCompletionCriterion) error {
	for _, criterion := range criteria {
		if criterion.Kind == models.TaskCompletionCriterionReview {
			return s.requireReviewStage(ctx, task.WorkflowID)
		}
	}
	return nil
}
