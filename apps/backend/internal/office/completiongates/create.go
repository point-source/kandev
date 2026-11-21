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
	case errors.Is(err, ErrCapAboveCeiling):
		return &taskservice.CompletionCriteriaRefusal{Code: "completion_cap_above_ceiling"}
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
	if spec.Kind == models.TaskCompletionCriterionReviewAtHead {
		if err := s.requireReviewStage(ctx, input.WorkflowID); err != nil {
			return models.TaskCompletionCriterion{}, err
		}
	}
	above := spec.MaxVerificationAttempts > settings.CeilingAttempts || spec.MaxReworkRounds > settings.CeilingRework
	criterion := models.TaskCompletionCriterion{
		ID: spec.Kind, Kind: spec.Kind,
		Config: models.TaskCompletionCriterionConfig{
			ProtectedPaths: spec.ProtectedPaths, MaxVerificationAttempts: spec.MaxVerificationAttempts,
			MaxReworkRounds: spec.MaxReworkRounds,
		},
		OwnerKind: input.Creator.Kind, OwnerID: input.Creator.ID,
	}
	if above && !person {
		return models.TaskCompletionCriterion{}, ErrCapAboveCeiling
	}
	if above {
		criterion.Config.CapsConfirmedBy = input.Creator.ID
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
			Kind: strings.TrimSpace(request.Kind), ProtectedPaths: request.ProtectedPaths,
			MaxVerificationAttempts: request.MaxVerificationAttempts, MaxReworkRounds: request.MaxReworkRounds,
		})
	}
	return specs
}

// unionSpecs keeps every default and adds requested kinds; a requested spec
// replaces a default of the same kind only by adding protected paths and
// caps, never by dropping them.
func unionSpecs(defaults, requested []officemodels.CompletionGateCriterionSpec) []officemodels.CompletionGateCriterionSpec {
	merged := append([]officemodels.CompletionGateCriterionSpec(nil), defaults...)
	for _, spec := range requested {
		index := indexOfKind(merged, spec.Kind)
		if index < 0 {
			merged = append(merged, spec)
			continue
		}
		merged[index].ProtectedPaths = append(merged[index].ProtectedPaths, spec.ProtectedPaths...)
		if spec.MaxVerificationAttempts > 0 {
			merged[index].MaxVerificationAttempts = spec.MaxVerificationAttempts
		}
		if spec.MaxReworkRounds > 0 {
			merged[index].MaxReworkRounds = spec.MaxReworkRounds
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
