package completiongates

import (
	"context"
	"errors"
	"strings"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/kandev/kandev/internal/authz"
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

// Control errors.
var (
	ErrCapAboveCeiling   = errors.New("completion cap is above the workspace ceiling")
	ErrNotCriterionOwner = errors.New("only the criterion owner or a CEO agent can change this criterion")
	ErrInvalidSettings   = errors.New("completion gate settings are invalid")
	ErrInvalidCriterion  = errors.New("completion criterion is invalid")
)

const maxCap = 1000

// ExtendRequest raises a criterion's caps and releases a cap hold.
type ExtendRequest struct {
	ExpectedCriterionRevision int64  `json:"expected_criterion_revision"`
	MaxVerificationAttempts   int    `json:"max_verification_attempts"`
	MaxReworkRounds           int    `json:"max_rework_rounds"`
	Reason                    string `json:"reason"`
}

// ExtendAsPerson raises caps on behalf of a person; values above the ceiling
// are recorded as confirmed by that person.
func (s *Service) ExtendAsPerson(ctx context.Context, taskID, criterionID string, request ExtendRequest) (*models.TaskCompletionGateSnapshot, error) {
	actorID, err := s.authorizeHuman(ctx, taskID)
	if err != nil {
		return nil, err
	}
	confirmedBy := ""
	if s.aboveCeiling(ctx, taskID, request) {
		confirmedBy = actorID
	}
	return s.extend(ctx, taskID, criterionID, request, "human", actorID, confirmedBy)
}

// ExtendAsAgent raises caps for the owner agent or a CEO agent, within the
// ceiling. The task's assignee is refused.
func (s *Service) ExtendAsAgent(ctx context.Context, taskID, criterionID, agentID string, request ExtendRequest) (*models.TaskCompletionGateSnapshot, error) {
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if agentID == "" || agentID == task.AssigneeAgentProfileID {
		RecordVerifierRefused("self_verification")
		return nil, ErrNotCriterionOwner
	}
	snapshot, err := s.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return nil, err
	}
	criterion := findCriterionByID(snapshot, criterionID)
	isOwner := criterion.OwnerKind == ownerKindAgent && criterion.OwnerID == agentID
	if criterion.ID == "" || (!isOwner && (s.agents == nil || s.agents.WorkspaceCEO(ctx, task.WorkspaceID) != agentID)) {
		return nil, ErrNotCriterionOwner
	}
	if s.aboveCeiling(ctx, taskID, request) {
		return nil, ErrCapAboveCeiling
	}
	if request.ExpectedCriterionRevision == 0 {
		request.ExpectedCriterionRevision = criterion.CriterionRevision
	}
	return s.extend(ctx, taskID, criterionID, request, ownerKindAgent, agentID, "")
}

func (s *Service) aboveCeiling(ctx context.Context, taskID string, request ExtendRequest) bool {
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return false
	}
	attempts, rework := s.caps.Ceilings(ctx, task.WorkspaceID)
	return request.MaxVerificationAttempts > attempts || request.MaxReworkRounds > rework
}

func (s *Service) extend(ctx context.Context, taskID, criterionID string, request ExtendRequest, actorKind, actorID, confirmedBy string) (*models.TaskCompletionGateSnapshot, error) {
	if request.MaxVerificationAttempts < 0 || request.MaxReworkRounds < 0 ||
		request.MaxVerificationAttempts > maxCap || request.MaxReworkRounds > maxCap ||
		(request.MaxVerificationAttempts == 0 && request.MaxReworkRounds == 0) {
		return nil, ErrInvalidCriterion
	}
	outcome, err := s.gates.ApplyTaskCompletionControl(ctx, models.TaskCompletionControlChange{
		TaskID: taskID, CriterionID: criterionID, CriterionRevision: request.ExpectedCriterionRevision,
		Action: models.TaskCompletionControlExtendCaps, MaxVerificationAttempts: request.MaxVerificationAttempts,
		MaxReworkRounds: request.MaxReworkRounds, CapsConfirmedBy: confirmedBy,
		ActorKind: actorKind, ActorID: actorID, Reason: strings.TrimSpace(request.Reason),
	})
	if err != nil {
		return nil, err
	}
	// A cap hold stops CI evaluation; re-check now, and route a now-unheld
	// failure back to the stage it was waiting to return to.
	background := context.WithoutCancel(ctx)
	s.CI.Schedule(background, taskID)
	if criterion := findCriterionByID(outcome.Snapshot, criterionID); criterion.ID != "" && criterion.HeldReason == "" {
		s.Router.RouteCriterion(background, taskID, criterion)
	}
	return outcome.Snapshot, nil
}

// GetSettings returns the workspace settings with shipped defaults applied.
func (s *Service) GetSettings(ctx context.Context, workspaceID string) (officemodels.CompletionGateSettings, error) {
	if err := s.gates.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceRead); err != nil {
		return officemodels.CompletionGateSettings{}, err
	}
	return s.effectiveSettings(ctx, workspaceID)
}

func (s *Service) effectiveSettings(ctx context.Context, workspaceID string) (officemodels.CompletionGateSettings, error) {
	settings := officemodels.CompletionGateSettings{}
	if s.settings != nil {
		stored, _, err := s.settings.GetCompletionGateSettings(ctx, workspaceID)
		if err != nil {
			return settings, err
		}
		settings = stored
	}
	settings.WorkspaceID = workspaceID
	if settings.DefaultCriteria == nil {
		settings.DefaultCriteria = []officemodels.CompletionGateCriterionSpec{}
	}
	settings.CeilingAttempts = ceilingOrDefault(settings.CeilingAttempts)
	settings.CeilingRework = ceilingOrDefault(settings.CeilingRework)
	settings.DefaultMaxAttempts = capOrDefault(settings.DefaultMaxAttempts)
	settings.DefaultMaxRework = capOrDefault(settings.DefaultMaxRework)
	return settings, nil
}

// UpdateSettings validates and stores workspace settings.
func (s *Service) UpdateSettings(ctx context.Context, workspaceID string, settings officemodels.CompletionGateSettings) (officemodels.CompletionGateSettings, error) {
	if err := s.gates.AuthorizeWorkspaceScope(ctx, workspaceID, authz.ScopeWorkspaceManage); err != nil {
		return officemodels.CompletionGateSettings{}, err
	}
	settings.WorkspaceID = workspaceID
	if err := validateSettings(settings); err != nil {
		return officemodels.CompletionGateSettings{}, err
	}
	if err := s.settings.SetCompletionGateSettings(ctx, settings); err != nil {
		return officemodels.CompletionGateSettings{}, err
	}
	return s.effectiveSettings(ctx, workspaceID)
}

func validateSettings(settings officemodels.CompletionGateSettings) error {
	for _, value := range []int{settings.DefaultMaxAttempts, settings.DefaultMaxRework, settings.CeilingAttempts, settings.CeilingRework} {
		if value < 1 || value > maxCap {
			return ErrInvalidSettings
		}
	}
	if settings.DefaultMaxAttempts > settings.CeilingAttempts || settings.DefaultMaxRework > settings.CeilingRework {
		return ErrInvalidSettings
	}
	seen := map[string]bool{}
	for _, spec := range settings.DefaultCriteria {
		if seen[spec.Kind] {
			return ErrInvalidSettings
		}
		seen[spec.Kind] = true
		if err := ValidateCriterionSpec(spec); err != nil {
			return err
		}
	}
	return nil
}

// ValidateCriterionSpec checks one requested or default criterion.
func ValidateCriterionSpec(spec officemodels.CompletionGateCriterionSpec) error {
	switch spec.Kind {
	case models.TaskCompletionCriterionCIAtHead:
	case models.TaskCompletionCriterionReviewAtHead:
		if len(spec.ProtectedPaths) > 0 {
			return ErrInvalidCriterion
		}
	default:
		return ErrInvalidCriterion
	}
	if spec.MaxVerificationAttempts < 0 || spec.MaxReworkRounds < 0 || spec.MaxVerificationAttempts > maxCap || spec.MaxReworkRounds > maxCap {
		return ErrInvalidCriterion
	}
	for _, pattern := range spec.ProtectedPaths {
		if strings.TrimSpace(pattern) == "" || !doublestar.ValidatePattern(strings.TrimSpace(pattern)) {
			return ErrInvalidProtectedPath
		}
	}
	return nil
}

// ErrInvalidProtectedPath reports an empty or malformed protected path.
var ErrInvalidProtectedPath = errors.New("protected path is invalid")

// InboxItems returns the completion-gate Inbox items of a workspace.
func (s *Service) InboxItems(ctx context.Context, workspaceID string) ([]*officemodels.InboxItem, error) {
	if s.held == nil {
		return nil, nil
	}
	held, err := s.held.ListHeldCompletionCriteria(ctx, workspaceID)
	if err != nil {
		return nil, err
	}
	return inboxItems(held), nil
}

// AssigneeRunHeld reports whether agentID is the assignee of a task that has
// a held criterion, so the scheduler skips that run.
func (s *Service) AssigneeRunHeld(ctx context.Context, taskID, agentID string) bool {
	if taskID == "" || agentID == "" {
		return false
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task.AssigneeAgentProfileID != agentID {
		return false
	}
	snapshot, err := s.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return false
	}
	for _, criterion := range snapshot.Criteria {
		if criterion.HeldReason != "" {
			return true
		}
	}
	return false
}

// EffectiveCaps is a criterion's resolved caps and how much of each is used.
type EffectiveCaps struct {
	CriterionID             string `json:"criterion_id"`
	MaxVerificationAttempts int    `json:"max_verification_attempts"`
	MaxReworkRounds         int    `json:"max_rework_rounds"`
	AttemptCount            int    `json:"attempt_count"`
	ReworkCount             int    `json:"rework_count"`
}

// EffectiveCaps resolves the caps of every host-verified criterion on a task.
func (s *Service) EffectiveCaps(ctx context.Context, taskID string) ([]EffectiveCaps, error) {
	if err := s.gates.AuthorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	snapshot, err := s.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return nil, err
	}
	caps := []EffectiveCaps{}
	for _, criterion := range snapshot.Criteria {
		if !models.IsHostVerifiedCriterionKind(criterion.Kind) {
			continue
		}
		attempts, rework := s.caps.Resolve(ctx, taskID, criterion)
		caps = append(caps, EffectiveCaps{
			CriterionID: criterion.ID, MaxVerificationAttempts: attempts, MaxReworkRounds: rework,
			AttemptCount: criterion.AttemptCount, ReworkCount: criterion.ReworkCount,
		})
	}
	return caps, nil
}
