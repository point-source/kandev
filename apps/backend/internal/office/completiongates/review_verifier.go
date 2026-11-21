package completiongates

import (
	"context"
	"errors"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

const (
	deciderTypeAgent         = "agent"
	decisionApproved         = "approved"
	decisionRejected         = "rejected"
	decisionChangesRequested = "changes_requested"
)

func isRejection(decision string) bool {
	return decision == decisionRejected || decision == decisionChangesRequested
}

// TaskReader reads the task's assignee.
type TaskReader interface {
	GetTask(ctx context.Context, taskID string) (*models.Task, error)
}

// ReviewStore reads reviewer seats and their decisions.
type ReviewStore interface {
	ListStepDecisions(ctx context.Context, taskID, stepID string) ([]*workflowmodels.WorkflowStepDecision, error)
	ListStepParticipantsForTask(ctx context.Context, stepID, taskID string) ([]*workflowmodels.WorkflowStepParticipant, error)
}

// ReviewVerifier pins review decisions to the PR heads and records
// review results. It implements the dashboard review hook.
type ReviewVerifier struct {
	gates   GateService
	prs     PRSource
	tasks   TaskReader
	reviews ReviewStore
	logger  *logger.Logger
	policy  EvaluationPolicy
}

// NewReviewVerifier creates a review verifier.
func NewReviewVerifier(gates GateService, prs PRSource, tasks TaskReader, reviews ReviewStore, log *logger.Logger) *ReviewVerifier {
	return &ReviewVerifier{gates: gates, prs: prs, tasks: tasks, reviews: reviews, logger: log}
}

// SetPolicy installs the routing policy consulted around each record.
func (v *ReviewVerifier) SetPolicy(policy EvaluationPolicy) { v.policy = policy }

// DecisionHeads returns the heads revision stored with a decision on a task
// with a review criterion. The assignee's own decisions get no heads.
func (v *ReviewVerifier) DecisionHeads(ctx context.Context, taskID, deciderType, deciderID string) (string, string) {
	if _, ok := v.reviewCriterion(ctx, taskID); !ok {
		return "", ""
	}
	if deciderType == deciderTypeAgent {
		task, err := v.tasks.GetTask(ctx, taskID)
		if err != nil {
			return "", ""
		}
		if task.AssigneeAgentProfileID != "" && task.AssigneeAgentProfileID == deciderID {
			RecordVerifierRefused("self_approval_ignored")
			return "", NoticeSelfApprovalIgnored
		}
	}
	links, err := v.prs.LinkedPRs(ctx, taskID)
	if err != nil || len(links) == 0 {
		return "", ""
	}
	return HeadsRevision(links), ""
}

// DecisionRecorded evaluates the review criterion after a decision commits.
func (v *ReviewVerifier) DecisionRecorded(ctx context.Context, taskID, stepID, _ string) {
	if err := v.evaluate(ctx, taskID, stepID); err != nil {
		v.logger.Warn("completion gate review evaluation failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

func (v *ReviewVerifier) reviewCriterion(ctx context.Context, taskID string) (models.TaskCompletionCriterion, bool) {
	snapshot, err := v.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return models.TaskCompletionCriterion{}, false
	}
	return findCriterionOfKind(snapshot, models.TaskCompletionCriterionReview)
}

// evaluate recomputes the review criterion from the decisions of the
// step's required reviewer seats: an active rejection fails it, and approval
// of the live heads by every required seat passes it. A person's send back
// stands until the heads move.
func (v *ReviewVerifier) evaluate(ctx context.Context, taskID, stepID string) error {
	criterion, ok := v.reviewCriterion(ctx, taskID)
	if !ok {
		return nil
	}
	links, err := v.prs.LinkedPRs(ctx, taskID)
	if err != nil || len(links) == 0 {
		return err
	}
	revision := HeadsRevision(links)
	if criterion.ResultReason == models.TaskCompletionReasonSentBack && criterion.ResultSubjectRevision == revision {
		return nil
	}
	seats, err := v.requiredSeats(ctx, taskID, stepID)
	if err != nil || len(seats) == 0 {
		return err
	}
	decisions, err := v.reviews.ListStepDecisions(ctx, taskID, stepID)
	if err != nil {
		return err
	}
	active := seatDecisions(latestActiveBySeat(decisions), seats)
	change := models.TaskCompletionEvaluationChange{
		TaskID: taskID, CriterionID: criterion.ID, CriterionRevision: criterion.CriterionRevision,
		SubjectRevision: revision, VerifierKind: VerifierKindReviews,
	}
	for _, decision := range active {
		if isRejection(decision.Decision) {
			change.Result, change.Reason = models.TaskCompletionResultFail, ReasonReviewRejected
			change.EvidenceReference = decision.ID
			return v.record(ctx, criterion, change)
		}
	}
	approvers, covered := coverage(seats, revision, active)
	if !covered {
		return nil
	}
	change.Result, change.Reason = models.TaskCompletionResultPass, ReasonReviewApproved
	change.VerifierID = approvers[len(approvers)-1].DeciderID
	change.EvidenceReference = decisionIDs(approvers)
	return v.record(ctx, criterion, change)
}

// requiredSeats returns the step's decision-required reviewer seats.
func (v *ReviewVerifier) requiredSeats(ctx context.Context, taskID, stepID string) (map[string]bool, error) {
	participants, err := v.reviews.ListStepParticipantsForTask(ctx, stepID, taskID)
	if err != nil {
		return nil, err
	}
	seats := map[string]bool{}
	for _, seat := range participants {
		if seat.Role == workflowmodels.ParticipantRoleReviewer && seat.DecisionRequired {
			seats[seat.ID] = true
		}
	}
	return seats, nil
}

// seatDecisions keeps the decisions recorded by the required seats.
func seatDecisions(decisions []*workflowmodels.WorkflowStepDecision, seats map[string]bool) []*workflowmodels.WorkflowStepDecision {
	kept := make([]*workflowmodels.WorkflowStepDecision, 0, len(decisions))
	for _, decision := range decisions {
		if seats[decision.ParticipantID] {
			kept = append(kept, decision)
		}
	}
	return kept
}

// latestActiveBySeat keeps each seat's newest unsuperseded decision, in
// decision order.
func latestActiveBySeat(decisions []*workflowmodels.WorkflowStepDecision) []*workflowmodels.WorkflowStepDecision {
	latest := map[string]*workflowmodels.WorkflowStepDecision{}
	for _, decision := range decisions {
		if decision.SupersededAt != nil {
			continue
		}
		if current := latest[decision.ParticipantID]; current == nil || !decision.DecidedAt.Before(current.DecidedAt) {
			latest[decision.ParticipantID] = decision
		}
	}
	active := make([]*workflowmodels.WorkflowStepDecision, 0, len(latest))
	for _, decision := range decisions {
		if latest[decision.ParticipantID] == decision {
			active = append(active, decision)
		}
	}
	return active
}

// LatestDecision returns the newest decision recorded at a step, superseded
// or not, so a rejection that moved the task is still identifiable.
func (v *ReviewVerifier) LatestDecision(ctx context.Context, taskID, stepID string) *workflowmodels.WorkflowStepDecision {
	decisions, err := v.reviews.ListStepDecisions(ctx, taskID, stepID)
	if err != nil {
		return nil
	}
	var latest *workflowmodels.WorkflowStepDecision
	for _, decision := range decisions {
		if latest == nil || !decision.DecidedAt.Before(latest.DecidedAt) {
			latest = decision
		}
	}
	return latest
}

// coverage reports whether every required seat approved the current heads.
// It returns the counting approvals.
func coverage(seats map[string]bool, revision string, active []*workflowmodels.WorkflowStepDecision) ([]*workflowmodels.WorkflowStepDecision, bool) {
	approved := map[string]bool{}
	var counting []*workflowmodels.WorkflowStepDecision
	for _, decision := range active {
		if decision.Decision == decisionApproved && decision.PRHeads == revision {
			approved[decision.ParticipantID] = true
			counting = append(counting, decision)
		}
	}
	for seat := range seats {
		if !approved[seat] {
			return nil, false
		}
	}
	return counting, len(counting) > 0
}

func decisionIDs(decisions []*workflowmodels.WorkflowStepDecision) string {
	ids := make([]string, len(decisions))
	for index, decision := range decisions {
		ids[index] = decision.ID
	}
	return strings.Join(ids, ",")
}

func (v *ReviewVerifier) record(ctx context.Context, criterion models.TaskCompletionCriterion, change models.TaskCompletionEvaluationChange) error {
	if change.VerifierID == "" {
		change.VerifierID = VerifierKindReviews
	}
	if v.policy != nil {
		v.policy.Prepare(ctx, change.TaskID, criterion, &change)
	}
	outcome, err := v.gates.RecordTaskCompletionEvaluation(ctx, change)
	if errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		return nil
	}
	if err != nil || !outcome.Changed {
		return err
	}
	recordEvaluationMetric(models.TaskCompletionCriterionReview, change.Result)
	if v.policy != nil {
		v.policy.AfterRecord(ctx, change.TaskID, outcome)
	}
	return nil
}
