package service

import (
	"context"
	"strings"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	completionGateActorSystem = "system"
	completionGateActorHuman  = "human"
)

// TaskCompletionHumanConfirmation authorizes one revision-bound weakening or
// removal of currently unmet criteria from the native task UI.
type TaskCompletionHumanConfirmation struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

// SetTaskCompletionCriteriaRequest replaces the criterion set under an
// observed revision. Plugins cannot provide HumanConfirmation because exact
// Host adapters use the plugin actor identity and reject that field.
type SetTaskCompletionCriteriaRequest struct {
	ExpectedRevision  int64                            `json:"expected_revision"`
	Criteria          []models.TaskCompletionCriterion `json:"criteria"`
	HumanConfirmation *TaskCompletionHumanConfirmation `json:"human_confirmation,omitempty"`
}

// VerifyTaskCompletionCriterionRequest submits typed evidence for one current
// criterion under its observed set revision.
type VerifyTaskCompletionCriterionRequest struct {
	ExpectedRevision int64                         `json:"expected_revision"`
	CriterionID      string                        `json:"criterion_id"`
	Evidence         models.TaskCompletionEvidence `json:"evidence"`
}

// TaskCompletionMoveOverrideRequest confirms one intended move in the native
// task UI. The authenticated identity, source, target, and set revision are
// bound by MoveTaskWithOptions before repository admission.
type TaskCompletionMoveOverrideRequest struct {
	ExpectedRevision int64  `json:"expected_revision"`
	Reason           string `json:"reason"`
}

// ExactTaskCompletionCriteriaRequest is the task-service command surface used
// only by the authorized plugin Host adapter.
type ExactTaskCompletionCriteriaRequest struct {
	WorkspaceID                 string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	OperationID                 string
	PayloadDigest               string
	ClaimFence                  repository.TaskManagementClaimFence
	ActorID                     string
	Criteria                    []models.TaskCompletionCriterion
}

// ExactTaskCompletionEvidenceRequest is the task-service command surface used
// only by the authorized plugin Host adapter.
type ExactTaskCompletionEvidenceRequest struct {
	WorkspaceID                 string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	OperationID                 string
	PayloadDigest               string
	ClaimFence                  repository.TaskManagementClaimFence
	ActorID                     string
	CriterionID                 string
	Evidence                    models.TaskCompletionEvidence
}

// ExactTaskCompletionGateResult includes the domain receipt replay marker.
type ExactTaskCompletionGateResult struct {
	Snapshot       *models.TaskCompletionGateSnapshot
	AlreadyApplied bool
}

// GetTaskCompletionGate returns the canonical gate and current blocker list.
func (s *Service) GetTaskCompletionGate(ctx context.Context, taskID string) (*models.TaskCompletionGateSnapshot, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if _, err := s.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.TaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	return repo.GetTaskCompletionGate(ctx, taskID)
}

// SetTaskCompletionCriteria stores an exact, versioned criterion set. A human
// confirmation is checked against the same set revision and actor used by the
// repository transaction.
func (s *Service) SetTaskCompletionCriteria(ctx context.Context, taskID string, request SetTaskCompletionCriteriaRequest) (*models.TaskCompletionGateSnapshot, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := guardCompletionGateSelfChange(ctx, task); err != nil {
		return nil, err
	}
	if err := s.validateCompletionCriteriaChange(ctx, task, request.Criteria); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.TaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	actorKind, actorID, human := taskCompletionActor(ctx)
	change := models.TaskCompletionCriteriaChange{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, ExpectedRevision: request.ExpectedRevision,
		Criteria: publicCompletionCriteria(task.ID, request.Criteria, actorID, human), ActorKind: actorKind, ActorID: actorID,
	}
	if request.HumanConfirmation != nil {
		if !human || strings.TrimSpace(request.HumanConfirmation.Reason) == "" {
			return nil, repoerrors.ErrTaskCompletionHumanConfirmationRequired
		}
		change.HumanConfirmationRevision = request.HumanConfirmation.ExpectedRevision
		change.HumanConfirmationReason = strings.TrimSpace(request.HumanConfirmation.Reason)
	}
	snapshot, err := repo.SetTaskCompletionCriteria(ctx, change)
	if err != nil {
		return nil, err
	}
	s.publishCompletionGateTaskUpdate(ctx, task.ID)
	return snapshot, nil
}

// SetTaskCompletionCriteriaExact applies an approved plugin command with
// resource-version and management-generation fencing in the repository.
func (s *Service) SetTaskCompletionCriteriaExact(ctx context.Context, taskID string, request ExactTaskCompletionCriteriaRequest) (*ExactTaskCompletionGateResult, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	if err := s.validateCompletionCriteriaChange(ctx, task, request.Criteria); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.ExactTaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	snapshot, alreadyApplied, err := repo.SetTaskCompletionCriteriaExact(ctx, models.TaskCompletionCriteriaChange{
		TaskID: taskID, WorkspaceID: request.WorkspaceID,
		ExpectedTaskResourceVersion: request.ExpectedTaskResourceVersion,
		ExpectedRevision:            request.ExpectedRevision, OperationID: request.OperationID,
		PayloadDigest: request.PayloadDigest, ClaimFence: request.ClaimFence,
		Criteria: publicCompletionCriteria(taskID, request.Criteria, request.ActorID, false), ActorKind: "plugin", ActorID: request.ActorID,
	})
	if err != nil {
		return nil, err
	}
	s.publishCompletionGateTaskUpdate(ctx, taskID)
	return &ExactTaskCompletionGateResult{Snapshot: snapshot, AlreadyApplied: alreadyApplied}, nil
}

// Default descriptions for host-verified criteria stored when the caller
// supplies none. They are task data, not UI copy.
var hostCriterionDescriptions = map[string]string{
	models.TaskCompletionCriterionCheck:  "CI passes at the PR head commit",       // i18n-exempt: stored task data
	models.TaskCompletionCriterionReview: "Review approved at the PR head commit", // i18n-exempt: stored task data
}

func sanitizeCompletionCriteria(criteria []models.TaskCompletionCriterion) []models.TaskCompletionCriterion {
	return sanitizeCompletionCriteriaForTask("", criteria)
}

// sanitizeCompletionCriteriaForTask keeps only creator-owned fields. Host
// criteria are bound to the task's PR heads subject when the task is known.
func sanitizeCompletionCriteriaForTask(taskID string, criteria []models.TaskCompletionCriterion) []models.TaskCompletionCriterion {
	clean := make([]models.TaskCompletionCriterion, len(criteria))
	for index, criterion := range criteria {
		next := models.TaskCompletionCriterion{
			ID: strings.TrimSpace(criterion.ID), Description: strings.TrimSpace(criterion.Description),
			Kind:            models.NormalizeTaskCompletionCriterionKind(strings.TrimSpace(criterion.Kind)),
			EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: criterion.EvidenceSubject.Kind, ID: criterion.EvidenceSubject.ID},
			Config:          criterion.Config, OwnerKind: criterion.OwnerKind, OwnerID: criterion.OwnerID,
		}
		if models.IsHostVerifiedCriterionKind(next.Kind) {
			if taskID != "" {
				next.EvidenceSubject = models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHeads, ID: taskID}
			}
			if next.Description == "" {
				next.Description = hostCriterionDescriptions[next.Kind]
			}
			if next.ID == "" {
				next.ID = next.Kind
			}
		}
		clean[index] = next
	}
	return clean
}

// publicCompletionCriteria sanitizes criteria from a caller-facing set
// command. Ownership always comes from the acting caller, and only a person
// confirms limits above the workspace ceiling, as themselves.
func publicCompletionCriteria(taskID string, criteria []models.TaskCompletionCriterion, actorID string, human bool) []models.TaskCompletionCriterion {
	clean := sanitizeCompletionCriteriaForTask(taskID, criteria)
	for index := range clean {
		clean[index].OwnerKind, clean[index].OwnerID = "", ""
		clean[index].Config.LimitsConfirmedBy = ""
		if human {
			clean[index].Config.LimitsConfirmedBy = actorID
		}
	}
	return clean
}

// prepareCreateCompletionCriteria sanitizes criteria supplied with a new task
// and records the creating actor as owner when the caller named none.
func prepareCreateCompletionCriteria(ctx context.Context, taskID string, criteria []models.TaskCompletionCriterion) []models.TaskCompletionCriterion {
	if len(criteria) == 0 {
		return nil
	}
	clean := sanitizeCompletionCriteriaForTask(taskID, criteria)
	actorKind, actorID, _ := taskCompletionActor(ctx)
	for index := range clean {
		if clean[index].OwnerKind == "" {
			clean[index].OwnerKind, clean[index].OwnerID = actorKind, actorID
		}
	}
	return clean
}

// guardCompletionGateSelfChange refuses gate changes from the task's own runs
// and from its assignee agent, independent of authentication.
func guardCompletionGateSelfChange(ctx context.Context, task *models.Task) error {
	caller, ok := models.CompletionGateCallerFromContext(ctx)
	if !ok {
		return nil
	}
	if caller.TaskID == task.ID || (caller.AgentProfileID != "" && caller.AgentProfileID == task.AssigneeAgentProfileID) {
		return repoerrors.ErrTaskCompletionSelfVerification
	}
	return nil
}

// RecordTaskCompletionEvaluation stores an in-process verifier result. It is
// called only by host verifiers and has no HTTP route.
func (s *Service) RecordTaskCompletionEvaluation(ctx context.Context, change models.TaskCompletionEvaluationChange) (*models.TaskCompletionEvaluationOutcome, error) {
	repo, ok := s.tasks.(repository.HostTaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	outcome, err := repo.RecordTaskCompletionEvaluation(ctx, change)
	if err != nil {
		return nil, err
	}
	if outcome.Changed {
		s.publishCompletionGateTaskUpdate(ctx, change.TaskID)
	}
	return outcome, nil
}

// ListTaskIDsWithUnmetCompletionCriteria lists open tasks with an unmet,
// unheld criterion of kind, for host verifier sweeps.
func (s *Service) ListTaskIDsWithUnmetCompletionCriteria(ctx context.Context, query models.TaskCompletionSweepQuery) ([]string, error) {
	repo, ok := s.tasks.(repository.HostTaskCompletionGateRepository)
	if !ok {
		return nil, nil
	}
	return repo.ListTaskIDsWithUnmetCompletionCriteria(ctx, query)
}

// ListHeldCompletionCriteria lists held criteria in a workspace for the
// Office Inbox. Internal callers only; the caller authorizes the workspace.
func (s *Service) ListHeldCompletionCriteria(ctx context.Context, workspaceID string) ([]models.HeldTaskCompletionCriterion, error) {
	repo, ok := s.tasks.(repository.HostTaskCompletionGateRepository)
	if !ok {
		return nil, nil
	}
	return repo.ListHeldCompletionCriteria(ctx, workspaceID)
}

// ApplyTaskCompletionControl applies a recovery action whose authority the
// caller has already established.
func (s *Service) ApplyTaskCompletionControl(ctx context.Context, change models.TaskCompletionControlChange) (*models.TaskCompletionControlOutcome, error) {
	repo, ok := s.tasks.(repository.HostTaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	outcome, err := repo.ApplyTaskCompletionControl(ctx, change)
	if err != nil {
		return nil, err
	}
	s.publishCompletionGateTaskUpdate(ctx, change.TaskID)
	return outcome, nil
}

// VerifyTaskCompletionCriterion records verifier identity and checks the
// subject revision before accepting evidence.
func (s *Service) VerifyTaskCompletionCriterion(ctx context.Context, taskID string, request VerifyTaskCompletionCriterionRequest) (*models.TaskCompletionGateSnapshot, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.TaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	actorKind, actorID, _ := taskCompletionActor(ctx)
	snapshot, err := repo.VerifyTaskCompletionCriterion(ctx, models.TaskCompletionEvidenceChange{
		TaskID: task.ID, WorkspaceID: task.WorkspaceID, ExpectedRevision: request.ExpectedRevision,
		CriterionID: request.CriterionID, Evidence: request.Evidence, ActorKind: actorKind, ActorID: actorID,
	})
	if err != nil {
		return nil, err
	}
	s.publishCompletionGateTaskUpdate(ctx, task.ID)
	return snapshot, nil
}

// VerifyTaskCompletionCriterionExact records a plugin verifier through the
// same typed evidence check and task-claim fence as native verification.
func (s *Service) VerifyTaskCompletionCriterionExact(ctx context.Context, taskID string, request ExactTaskCompletionEvidenceRequest) (*ExactTaskCompletionGateResult, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.ExactTaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	snapshot, alreadyApplied, err := repo.VerifyTaskCompletionCriterionExact(ctx, models.TaskCompletionEvidenceChange{
		TaskID: taskID, WorkspaceID: request.WorkspaceID,
		ExpectedTaskResourceVersion: request.ExpectedTaskResourceVersion,
		ExpectedRevision:            request.ExpectedRevision, OperationID: request.OperationID,
		PayloadDigest: request.PayloadDigest, ClaimFence: request.ClaimFence,
		CriterionID: request.CriterionID, Evidence: request.Evidence,
		ActorKind: "plugin", ActorID: request.ActorID,
	})
	if err != nil {
		return nil, err
	}
	s.publishCompletionGateTaskUpdate(ctx, taskID)
	return &ExactTaskCompletionGateResult{Snapshot: snapshot, AlreadyApplied: alreadyApplied}, nil
}

// ListTaskCompletionGateHistory returns the append-only native audit trail.
func (s *Service) ListTaskCompletionGateHistory(ctx context.Context, taskID string) ([]*models.TaskCompletionGateHistory, error) {
	if err := s.authorizeTaskScope(ctx, taskID, authz.ScopeWorkspaceRead); err != nil {
		return nil, err
	}
	if _, err := s.tasks.GetTask(ctx, taskID); err != nil {
		return nil, err
	}
	repo, ok := s.tasks.(repository.TaskCompletionGateRepository)
	if !ok {
		return nil, repoerrors.ErrTaskCompletionGateBlocked
	}
	return repo.ListTaskCompletionGateHistory(ctx, taskID)
}

func taskCompletionActor(ctx context.Context) (kind, id string, human bool) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.UserID) == "" {
		return completionGateActorSystem, completionGateActorSystem, false
	}
	return completionGateActorHuman, identity.UserID, true
}

func (s *Service) publishCompletionGateTaskUpdate(ctx context.Context, taskID string) {
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	s.publishTaskEvent(ctx, events.TaskUpdated, task, nil)
}
