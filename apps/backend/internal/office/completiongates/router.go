package completiongates

import (
	"context"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/task/models"
)

// DefaultCap is the shipped default for both caps.
const DefaultCap = 3

const reasonHeadMoved = "head_moved"

// Placement is where a task stands in its workflow, by step position.
type Placement struct {
	WorkflowID      string
	CurrentStepID   string
	CurrentPosition int
	WorkStepID      string
	WorkPosition    int
	ReviewStepID    string
	ReviewPosition  int
	Terminal        bool
}

func (p Placement) pastWork() bool {
	return !p.Terminal && p.WorkStepID != "" && p.CurrentPosition > p.WorkPosition
}

func (p Placement) pastReview() bool {
	return !p.Terminal && p.ReviewStepID != "" && p.CurrentPosition > p.ReviewPosition
}

// Navigator reads task placement and moves tasks through the normal task
// move API so admission, ledgers and events stay consistent.
type Navigator interface {
	TaskPlacement(ctx context.Context, taskID string) (Placement, error)
	MoveTask(ctx context.Context, taskID, workflowID, stepID string) error
	// RetryCompletingTransition re-applies a satisfied guarded transition of
	// stepID only when it enters the workflow's completing step.
	RetryCompletingTransition(ctx context.Context, taskID, workflowID, stepID, operationID string) error
}

// CapResolver returns the effective attempt and rework caps of a criterion.
type CapResolver func(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion) (attempts, rework int)

// HoldEscalator escalates a newly held criterion once.
type HoldEscalator interface {
	EscalateHold(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion)
}

// Router turns recorded results into workflow movement.
type Router struct {
	gates     GateService
	prs       PRSource
	navigator Navigator
	review    *ReviewVerifier
	caps      CapResolver
	escalator HoldEscalator
	logger    *logger.Logger
}

// NewRouter creates a router. Caps default to the criterion values, then 3.
func NewRouter(gates GateService, prs PRSource, navigator Navigator, review *ReviewVerifier, log *logger.Logger) *Router {
	return &Router{gates: gates, prs: prs, navigator: navigator, review: review, caps: criterionCaps, logger: log}
}

// SetNavigator installs the workflow navigator. Routing is inactive until set.
func (r *Router) SetNavigator(navigator Navigator) { r.navigator = navigator }

// SetCapResolver replaces the cap resolution.
func (r *Router) SetCapResolver(resolver CapResolver) { r.caps = resolver }

// SetEscalator installs the hold escalation.
func (r *Router) SetEscalator(escalator HoldEscalator) { r.escalator = escalator }

func criterionCaps(_ context.Context, _ string, criterion models.TaskCompletionCriterion) (int, int) {
	return capOrDefault(criterion.Config.MaxVerificationAttempts), capOrDefault(criterion.Config.MaxReworkRounds)
}

func capOrDefault(value int) int {
	if value > 0 {
		return value
	}
	return DefaultCap
}

// Prepare counts verification attempts and holds at the attempt cap.
func (r *Router) Prepare(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion, change *models.TaskCompletionEvaluationChange) {
	if change.Result != models.TaskCompletionResultCannotEvaluate {
		return
	}
	attempts, _ := r.caps(ctx, taskID, criterion)
	if criterion.AttemptCount >= attempts {
		change.Hold = HoldAttemptCapReached
		return
	}
	change.CountAttempt = true
}

// AfterRecord routes a changed result.
func (r *Router) AfterRecord(ctx context.Context, taskID string, outcome *models.TaskCompletionEvaluationOutcome) {
	criterion := outcome.Criterion
	if criterion.HeldReason != "" && criterion.HeldAt != nil && criterion.ResultAt != nil && !criterion.HeldAt.Before(*criterion.ResultAt) {
		r.escalate(ctx, taskID, criterion)
	}
	if criterion.Result == models.TaskCompletionResultPass && outcome.Snapshot != nil && !outcome.Snapshot.Blocked {
		r.retryCompletion(ctx, taskID, criterion.ResultSubjectRevision)
		return
	}
	r.RouteCriterion(ctx, taskID, criterion)
}

// RouteCriterion returns the task for an unmet criterion that is not held: a
// CI failure goes back to the work stage, a review left stale by a push goes
// back to the review stage. Keys match the original routing so a return that
// was held at the rework cap is applied once the cap is raised.
func (r *Router) RouteCriterion(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion) {
	if criterion.HeldReason != "" || criterion.ResultSubjectRevision == "" {
		return
	}
	switch {
	case criterion.Kind == models.TaskCompletionCriterionCIAtHead && criterion.Result == models.TaskCompletionResultFail:
		r.returnTo(ctx, taskID, criterion, "ci:"+criterion.ResultSubjectRevision, func(p Placement) (string, bool) {
			return p.WorkStepID, p.pastWork()
		})
	case criterion.Kind == models.TaskCompletionCriterionReviewAtHead && criterion.ResultReason == reasonHeadMoved:
		r.returnTo(ctx, taskID, criterion, "head:"+criterion.ResultSubjectRevision, func(p Placement) (string, bool) {
			return p.ReviewStepID, p.pastReview()
		})
	}
}

func (r *Router) escalate(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion) {
	recordHoldMetric(criterion.HeldReason)
	if r.escalator != nil {
		r.escalator.EscalateHold(ctx, taskID, criterion)
	}
}

// returnTo counts one rework round under key and moves the task to the step
// chosen by target, or holds the criterion at the rework cap.
func (r *Router) returnTo(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion, key string, target func(Placement) (string, bool)) {
	placement, ok := r.placement(ctx, taskID)
	if !ok {
		return
	}
	stepID, ok := target(placement)
	if !ok || stepID == "" || !r.claimRework(ctx, taskID, criterion, key) {
		return
	}
	r.move(ctx, taskID, placement.WorkflowID, stepID)
}

// sendBack counts a person's send-back as one rework round and returns the
// task to the work stage. It reports true when the round was counted while
// the task was already in the work stage, so no move wakes the assignee.
func (r *Router) sendBack(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion, key string) bool {
	placement, ok := r.placement(ctx, taskID)
	if !ok || placement.Terminal || placement.WorkStepID == "" || !r.claimRework(ctx, taskID, criterion, key) {
		return false
	}
	if placement.CurrentStepID == placement.WorkStepID {
		return true
	}
	r.move(ctx, taskID, placement.WorkflowID, placement.WorkStepID)
	return false
}

// placement reads the task's workflow position; routing is off without a navigator.
func (r *Router) placement(ctx context.Context, taskID string) (Placement, bool) {
	if r.navigator == nil {
		return Placement{}, false
	}
	placement, err := r.navigator.TaskPlacement(ctx, taskID)
	if err != nil {
		r.logger.Warn("completion gate placement read failed", zap.String("task_id", taskID), zap.Error(err))
		return Placement{}, false
	}
	return placement, true
}

// claimRework counts one keyed rework round and reports whether it is new.
// Reaching the cap holds and escalates the criterion instead.
func (r *Router) claimRework(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion, key string) bool {
	after, counted, ok := r.countRework(ctx, taskID, criterion, key)
	if !ok {
		return false
	}
	if !counted {
		if after.HeldReason == HoldReworkCapReached && criterion.HeldReason != HoldReworkCapReached {
			r.escalate(ctx, taskID, after)
		}
		return false
	}
	recordReworkMetric(criterion.Kind)
	return true
}

func (r *Router) move(ctx context.Context, taskID, workflowID, stepID string) {
	if err := r.navigator.MoveTask(ctx, taskID, workflowID, stepID); err != nil {
		r.logger.Warn("completion gate return move failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

// countRework applies a keyed rework count. counted reports a new round.
func (r *Router) countRework(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion, key string) (models.TaskCompletionCriterion, bool, bool) {
	if current, err := r.gates.GetTaskCompletionGate(ctx, taskID); err == nil {
		if fresh := findCriterionByID(current, criterion.ID); fresh.ID != "" {
			criterion = fresh
		}
	}
	_, rework := r.caps(ctx, taskID, criterion)
	outcome, err := r.gates.ApplyTaskCompletionControl(ctx, models.TaskCompletionControlChange{
		TaskID: taskID, CriterionID: criterion.ID, CriterionRevision: criterion.CriterionRevision,
		Action: models.TaskCompletionControlCountRework, Key: key, ReworkCap: rework,
		ActorKind: "system", ActorID: "completion_gate_router",
	})
	if err != nil {
		r.logger.Warn("completion gate rework count failed", zap.String("task_id", taskID), zap.Error(err))
		return criterion, false, false
	}
	return findCriterionByID(outcome.Snapshot, criterion.ID), outcome.Claimed, true
}

func (r *Router) retryCompletion(ctx context.Context, taskID, revision string) {
	if r.navigator == nil {
		return
	}
	placement, err := r.navigator.TaskPlacement(ctx, taskID)
	if err != nil || placement.Terminal || placement.CurrentStepID == "" {
		return
	}
	operationID := "completion-gate:" + taskID + ":" + placement.CurrentStepID + ":" + revision
	if err := r.navigator.RetryCompletingTransition(ctx, taskID, placement.WorkflowID, placement.CurrentStepID, operationID); err != nil {
		r.logger.Warn("completion gate transition retry failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

// OnTaskMoved routes a task that left the work or review stage while a
// criterion is unmet. transitionKey identifies the move.
func (r *Router) OnTaskMoved(ctx context.Context, taskID, fromStepID, transitionKey string) {
	if r.navigator == nil {
		return
	}
	snapshot, err := r.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil || snapshot == nil || len(snapshot.Criteria) == 0 {
		return
	}
	placement, err := r.navigator.TaskPlacement(ctx, taskID)
	if err != nil {
		return
	}
	key := "move:" + transitionKey
	if r.routeCIOnMove(ctx, taskID, fromStepID, key, snapshot, placement) {
		return
	}
	r.routeReviewOnMove(ctx, taskID, fromStepID, key, snapshot, placement)
}

func (r *Router) routeCIOnMove(ctx context.Context, taskID, fromStepID, key string, snapshot *models.TaskCompletionGateSnapshot, placement Placement) bool {
	ci, ok := findCriterionOfKind(snapshot, models.TaskCompletionCriterionCIAtHead)
	if !ok || fromStepID != placement.WorkStepID || !placement.pastWork() || !r.failingAtLiveHeads(ctx, taskID, ci) {
		return false
	}
	r.returnTo(ctx, taskID, ci, key, func(p Placement) (string, bool) { return p.WorkStepID, p.pastWork() })
	return true
}

func (r *Router) routeReviewOnMove(ctx context.Context, taskID, fromStepID, key string, snapshot *models.TaskCompletionGateSnapshot, placement Placement) {
	review, ok := findCriterionOfKind(snapshot, models.TaskCompletionCriterionReviewAtHead)
	if !ok || placement.ReviewStepID == "" || fromStepID != placement.ReviewStepID {
		return
	}
	if placement.CurrentStepID == placement.WorkStepID {
		if r.review == nil {
			return
		}
		if latest := r.review.LatestDecision(ctx, taskID, fromStepID); latest != nil && isRejection(latest.Decision) {
			r.countRejection(ctx, taskID, review, "rejection:"+latest.ID)
		}
		return
	}
	if placement.pastReview() && !r.reviewPassing(ctx, taskID, fromStepID, review) {
		r.returnTo(ctx, taskID, review, key, func(p Placement) (string, bool) { return p.ReviewStepID, p.pastReview() })
	}
}

func (r *Router) countRejection(ctx context.Context, taskID string, review models.TaskCompletionCriterion, key string) {
	after, counted, ok := r.countRework(ctx, taskID, review, key)
	if !ok {
		return
	}
	if counted {
		recordReworkMetric(review.Kind)
		return
	}
	if after.HeldReason == HoldReworkCapReached && review.HeldReason != HoldReworkCapReached {
		r.escalate(ctx, taskID, after)
	}
}

func (r *Router) failingAtLiveHeads(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion) bool {
	if criterion.Result != models.TaskCompletionResultFail {
		return false
	}
	links, err := r.prs.LinkedPRs(ctx, taskID)
	return err == nil && len(links) > 0 && HeadsRevision(links) == criterion.ResultSubjectRevision
}

// reviewPassing evaluates the review step the task just left before deciding
// it is stale, because the quorum transition commits before the decision
// hook records the pass.
func (r *Router) reviewPassing(ctx context.Context, taskID, reviewStepID string, review models.TaskCompletionCriterion) bool {
	if r.review != nil {
		_ = r.review.evaluate(ctx, taskID, reviewStepID)
	}
	snapshot, err := r.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return true
	}
	for _, blocker := range snapshot.Blockers {
		if blocker.CriterionID == review.ID {
			return false
		}
	}
	return true
}

// OnHeadsChanged marks a passed review stale when the heads moved and returns
// a task that is past the review stage.
func (r *Router) OnHeadsChanged(ctx context.Context, taskID string) {
	snapshot, err := r.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return
	}
	review, ok := findCriterionOfKind(snapshot, models.TaskCompletionCriterionReviewAtHead)
	if !ok || review.Result != models.TaskCompletionResultPass {
		return
	}
	links, err := r.prs.LinkedPRs(ctx, taskID)
	if err != nil || len(links) == 0 {
		return
	}
	revision := HeadsRevision(links)
	if revision == review.ResultSubjectRevision {
		return
	}
	outcome, err := r.gates.RecordTaskCompletionEvaluation(ctx, models.TaskCompletionEvaluationChange{
		TaskID: taskID, CriterionID: review.ID, CriterionRevision: review.CriterionRevision,
		SubjectRevision: revision, Result: models.TaskCompletionResultPending, Reason: reasonHeadMoved,
		VerifierKind: VerifierKindReviews, VerifierID: VerifierKindReviews,
	})
	if err != nil || !outcome.Changed {
		return
	}
	r.returnTo(ctx, taskID, outcome.Criterion, "head:"+revision, func(p Placement) (string, bool) {
		return p.ReviewStepID, p.pastReview()
	})
}

func findCriterionByID(snapshot *models.TaskCompletionGateSnapshot, criterionID string) models.TaskCompletionCriterion {
	if snapshot != nil {
		for _, criterion := range snapshot.Criteria {
			if criterion.ID == criterionID {
				return criterion
			}
		}
	}
	return models.TaskCompletionCriterion{}
}
