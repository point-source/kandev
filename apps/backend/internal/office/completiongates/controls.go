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
	ErrLimitAboveCeiling = errors.New("completion limit is above the workspace ceiling")
	ErrNotCriterionOwner = errors.New("only the criterion owner or a CEO agent can change this criterion")
	ErrInvalidSettings   = errors.New("completion gate settings are invalid")
	ErrInvalidCriterion  = errors.New("completion criterion is invalid")
)

const maxLimit = 1000

// ExtendRequest raises a criterion's limits and releases a limit hold.
type ExtendRequest struct {
	ExpectedCriterionRevision int64  `json:"expected_criterion_revision"`
	VerificationLimit         int    `json:"verification_limit"`
	ReworkLimit               int    `json:"rework_limit"`
	Reason                    string `json:"reason"`
}

// ExtendAsPerson raises limits on behalf of a person; values above the ceiling
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

// ExtendAsAgent raises limits for the owner agent or a CEO agent, within the
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
		return nil, ErrLimitAboveCeiling
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
	attempts, rework := s.limits.Ceilings(ctx, task.WorkspaceID)
	return request.VerificationLimit > attempts || request.ReworkLimit > rework
}

func (s *Service) extend(ctx context.Context, taskID, criterionID string, request ExtendRequest, actorKind, actorID, confirmedBy string) (*models.TaskCompletionGateSnapshot, error) {
	if request.VerificationLimit < 0 || request.ReworkLimit < 0 ||
		request.VerificationLimit > maxLimit || request.ReworkLimit > maxLimit ||
		(request.VerificationLimit == 0 && request.ReworkLimit == 0) {
		return nil, ErrInvalidCriterion
	}
	outcome, err := s.gates.ApplyTaskCompletionControl(ctx, models.TaskCompletionControlChange{
		TaskID: taskID, CriterionID: criterionID, CriterionRevision: request.ExpectedCriterionRevision,
		Action: models.TaskCompletionControlExtendLimits, VerificationLimit: request.VerificationLimit,
		ReworkLimit: request.ReworkLimit, LimitsConfirmedBy: confirmedBy,
		ActorKind: actorKind, ActorID: actorID, Reason: strings.TrimSpace(request.Reason),
	})
	if err != nil {
		return nil, err
	}
	// A limit hold stops CI evaluation; re-check now, and route a now-unheld
	// failure back to the stage it was waiting to return to.
	background := context.WithoutCancel(ctx)
	s.CI.Schedule(background, taskID)
	criterion := findCriterionByID(outcome.Snapshot, criterionID)
	switch {
	case criterion.ID == "" || criterion.HoldKind != "":
	case criterion.ResultReason == models.TaskCompletionReasonSentBack && len(criterion.ResultDetail) > 0:
		s.routeSendBack(background, taskID, criterion, criterion.ResultDetail[0])
	default:
		s.Router.RouteCriterion(background, taskID, criterion)
	}
	s.Router.EvaluateApproval(background, taskID)
	return s.gates.GetTaskCompletionGate(ctx, taskID)
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
	settings.VerificationLimitCeiling = ceilingOrDefault(settings.VerificationLimitCeiling)
	settings.ReworkLimitCeiling = ceilingOrDefault(settings.ReworkLimitCeiling)
	settings.DefaultVerificationLimit = limitOrDefault(settings.DefaultVerificationLimit)
	settings.DefaultReworkLimit = limitOrDefault(settings.DefaultReworkLimit)
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
	for _, value := range []int{settings.DefaultVerificationLimit, settings.DefaultReworkLimit, settings.VerificationLimitCeiling, settings.ReworkLimitCeiling} {
		if value < 1 || value > maxLimit {
			return ErrInvalidSettings
		}
	}
	if settings.DefaultVerificationLimit > settings.VerificationLimitCeiling || settings.DefaultReworkLimit > settings.ReworkLimitCeiling ||
		settings.DefaultReviewWaitLimit < 0 || settings.DefaultReviewWaitLimit > MaxReviewWaitLimit {
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
	if !validSpecKind(spec) || !validSpecLimits(spec) {
		return ErrInvalidCriterion
	}
	for _, pattern := range spec.ProtectedPaths {
		if strings.TrimSpace(pattern) == "" || !doublestar.ValidatePattern(strings.TrimSpace(pattern)) {
			return ErrInvalidProtectedPath
		}
	}
	return nil
}

// validSpecKind accepts a check naming the github runner without a review wait
// limit, a review naming no runner and protecting no paths, and an approval
// carrying only a rework limit.
func validSpecKind(spec officemodels.CompletionGateCriterionSpec) bool {
	switch spec.Kind {
	case models.TaskCompletionCriterionCheck:
		return spec.Runner == models.TaskCompletionCheckRunnerGitHub && spec.ReviewWaitLimit == 0
	case models.TaskCompletionCriterionReview:
		return spec.Runner == "" && len(spec.ProtectedPaths) == 0
	case models.TaskCompletionCriterionApproval:
		return spec.Runner == "" && len(spec.ProtectedPaths) == 0 && spec.VerificationLimit == 0 && spec.ReviewWaitLimit == 0
	}
	return false
}

func validSpecLimits(spec officemodels.CompletionGateCriterionSpec) bool {
	return spec.VerificationLimit >= 0 && spec.ReworkLimit >= 0 && spec.VerificationLimit <= maxLimit &&
		spec.ReworkLimit <= maxLimit && spec.ReviewWaitLimit >= 0 && spec.ReviewWaitLimit <= MaxReviewWaitLimit
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

// AssigneeStartHeld counts a rejection that returned the task to the work
// stage, then reports whether the assignee's start there is held.
func (s *Service) AssigneeStartHeld(ctx context.Context, taskID, agentID string) bool {
	if s.Router != nil {
		s.Router.SettleWorkEntry(ctx, taskID)
	}
	return s.AssigneeRunHeld(ctx, taskID, agentID)
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
		if criterion.HoldKind != "" {
			return true
		}
	}
	return false
}

// EffectiveLimits is a criterion's resolved limits and how much of each is used.
type EffectiveLimits struct {
	CriterionID       string `json:"criterion_id"`
	VerificationLimit int    `json:"verification_limit"`
	ReworkLimit       int    `json:"rework_limit"`
	AttemptCount      int    `json:"attempt_count"`
	ReworkCount       int    `json:"rework_count"`
}

// EffectiveLimits resolves the limits of every host-verified criterion on a task.
func (s *Service) EffectiveLimits(ctx context.Context, taskID string) ([]EffectiveLimits, error) {
	if err := s.gates.AuthorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	snapshot, err := s.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return nil, err
	}
	limits := []EffectiveLimits{}
	for _, criterion := range snapshot.Criteria {
		if !models.IsHostVerifiedCriterionKind(criterion.Kind) {
			continue
		}
		attempts, rework := s.limits.Resolve(ctx, taskID, criterion)
		limits = append(limits, EffectiveLimits{
			CriterionID: criterion.ID, VerificationLimit: attempts, ReworkLimit: rework,
			AttemptCount: criterion.AttemptCount, ReworkCount: criterion.ReworkCount,
		})
	}
	return limits, nil
}
