package completiongates

import (
	"context"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// Causes of a no_reviewer_available hold, recorded as its detail.
const (
	// CauseAssigneeOnly: the only reviewer seat went to the task's assignee,
	// whose approvals never count.
	CauseAssigneeOnly = "assignee_only"
	// CauseNoReviewerRole: no agent can fill the reviewer role.
	CauseNoReviewerRole = "no_reviewer_role"
	// CauseAuthorProviderOnly: the author's provider is the only provider in
	// the reviewer run's effective order.
	CauseAuthorProviderOnly = "author_provider_only"
)

// ReasonWaitingForReviewer is the pending reason of a review whose seat run
// is parked until another provider recovers. Its detail is the park status,
// the earliest retry time and the time the wait began.
const ReasonWaitingForReviewer = "waiting_for_reviewer"

const reviewerActor = "completion_gate_reviewers"

// ReviewerCast reacts to a reviewer seat cast on review-stage entry. An
// unfillable seat or a seat that went to the assignee can never approve, so
// the review holds at once; any other seat clears such a hold.
func (s *Service) ReviewerCast(ctx context.Context, taskID, agentProfileID string, unfillable bool) {
	review, ok := s.reviewOf(ctx, taskID)
	if !ok {
		return
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	switch {
	case unfillable:
		s.holdReview(ctx, taskID, review, HoldNoReviewerAvailable, CauseNoReviewerRole)
	case agentProfileID != "" && agentProfileID == task.AssigneeAgentProfileID:
		s.holdReview(ctx, taskID, review, HoldNoReviewerAvailable, CauseAssigneeOnly)
	default:
		s.releaseReviewerHolds(ctx, taskID, review)
	}
}

// ReviewSeatParked reacts to a reviewer seat run parked by the author-provider
// exclusion. When the author's provider is the only one, no reviewer can ever
// run and the review holds at once. Otherwise the review waits, with the park
// visible, until the run resumes or an optional review wait limit runs out.
func (s *Service) ReviewSeatParked(ctx context.Context, taskID string, onlyAuthorProvider bool, status string, retryAt time.Time) {
	review, ok := s.reviewOf(ctx, taskID)
	if !ok {
		return
	}
	if onlyAuthorProvider {
		s.holdReview(ctx, taskID, review, HoldNoReviewerAvailable, CauseAuthorProviderOnly)
		return
	}
	if review.ResultReason == ReasonWaitingForReviewer {
		return
	}
	retry := ""
	if !retryAt.IsZero() {
		retry = retryAt.UTC().Format(time.RFC3339)
	}
	s.recordReviewPending(ctx, taskID, review, ReasonWaitingForReviewer,
		[]string{status, retry, time.Now().UTC().Format(time.RFC3339)})
}

// ReviewSeatLaunched clears a reviewer wait or hold once a seat run starts.
func (s *Service) ReviewSeatLaunched(ctx context.Context, taskID string) {
	review, ok := s.reviewOf(ctx, taskID)
	if !ok {
		return
	}
	s.releaseReviewerHolds(ctx, taskID, review)
	if review.ResultReason == ReasonWaitingForReviewer {
		s.recordReviewPending(ctx, taskID, review, "", nil)
	}
}

// sweepReviewWaits holds every review that has waited for a parked reviewer
// longer than its review wait limit. Reviews without a limit wait.
func (s *Service) sweepReviewWaits(ctx context.Context, now time.Time) {
	s.forEachUnmetReview(ctx, "", func(taskID string) { s.holdExpiredReviewWait(ctx, taskID, now) })
}

// forEachUnmetReview visits, in task order, every task whose review is unmet,
// limited to reviews with the given hold kind when one is set.
func (s *Service) forEachUnmetReview(ctx context.Context, holdKind string, visit func(taskID string)) {
	after := ""
	for {
		ids, err := s.gates.ListTaskIDsWithUnmetCompletionCriteria(ctx, models.TaskCompletionSweepQuery{
			Kind: models.TaskCompletionCriterionReview, HoldKind: holdKind, AfterTaskID: after, Limit: sweepBatchSize,
		})
		if err != nil || len(ids) == 0 {
			return
		}
		for _, taskID := range ids {
			visit(taskID)
		}
		if len(ids) < sweepBatchSize {
			return
		}
		after = ids[len(ids)-1]
	}
}

// ReviewerSeater seats a reviewer that can approve a task's review, in place
// of the assignee's fallback seat or the missing seat, and starts its review.
// It reports false when the only candidate is still the assignee or none.
type ReviewerSeater interface {
	SeatReviewer(ctx context.Context, taskID string) (bool, error)
}

// sweepReviewerSeats retries seating for reviews held because no seat could
// approve (assignee only, or no reviewer role), so a reviewer agent added or
// activated later clears the hold without a person.
func (s *Service) sweepReviewerSeats(ctx context.Context) {
	if s.seater == nil {
		return
	}
	s.forEachUnmetReview(ctx, HoldNoReviewerAvailable, func(taskID string) { s.reseatReviewer(ctx, taskID) })
}

func (s *Service) reseatReviewer(ctx context.Context, taskID string) {
	review, ok := s.reviewOf(ctx, taskID)
	if !ok || review.HoldKind != HoldNoReviewerAvailable || len(review.ResultDetail) == 0 ||
		(review.ResultDetail[0] != CauseAssigneeOnly && review.ResultDetail[0] != CauseNoReviewerRole) {
		return
	}
	seated, err := s.seater.SeatReviewer(ctx, taskID)
	if err != nil {
		s.logger.Warn("completion gate reviewer reseat failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if seated {
		s.releaseReviewerHolds(ctx, taskID, review)
	}
}

func (s *Service) holdExpiredReviewWait(ctx context.Context, taskID string, now time.Time) {
	review, ok := s.reviewOf(ctx, taskID)
	if !ok || review.HoldKind != "" || review.ResultReason != ReasonWaitingForReviewer || len(review.ResultDetail) < 3 {
		return
	}
	wait := s.limits.ReviewWait(ctx, taskID, review)
	since, err := time.Parse(time.RFC3339, review.ResultDetail[2])
	if wait <= 0 || err != nil || now.Sub(since) < wait {
		return
	}
	s.holdReview(ctx, taskID, review, HoldReviewWaitLimitReached, review.ResultDetail[2])
}

func (s *Service) reviewOf(ctx context.Context, taskID string) (models.TaskCompletionCriterion, bool) {
	snapshot, err := s.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return models.TaskCompletionCriterion{}, false
	}
	return findCriterionOfKind(snapshot, models.TaskCompletionCriterionReview)
}

// holdReview holds the review at the live heads, so a person accepts or sends
// back the commit they are shown, and escalates a new hold once.
func (s *Service) holdReview(ctx context.Context, taskID string, review models.TaskCompletionCriterion, kind, cause string) {
	heads := ""
	if links, err := s.prs.LinkedPRs(ctx, taskID); err == nil && len(links) > 0 {
		heads = HeadsRevision(links)
	}
	change := models.TaskCompletionControlChange{
		TaskID: taskID, CriterionID: review.ID, CriterionRevision: review.CriterionRevision,
		Action: models.TaskCompletionControlHold, HoldKind: kind, Detail: []string{cause}, SubjectRevision: heads,
		ActorKind: "system", ActorID: reviewerActor,
	}
	if kind == HoldReviewWaitLimitReached {
		// The reviewer may have launched since the sweep read the wait.
		change.RequireResultReason = ReasonWaitingForReviewer
	}
	outcome, err := s.gates.ApplyTaskCompletionControl(ctx, change)
	if err != nil {
		s.logger.Warn("completion gate review hold failed", zap.String("task_id", taskID), zap.String("hold_kind", kind), zap.Error(err))
		return
	}
	if outcome.Claimed {
		s.Router.escalate(ctx, taskID, findCriterionByID(outcome.Snapshot, review.ID))
	}
}

func (s *Service) releaseReviewerHolds(ctx context.Context, taskID string, review models.TaskCompletionCriterion) {
	if review.HoldKind != HoldNoReviewerAvailable && review.HoldKind != HoldReviewWaitLimitReached {
		return
	}
	if _, err := s.gates.ApplyTaskCompletionControl(ctx, models.TaskCompletionControlChange{
		TaskID: taskID, CriterionID: review.ID, CriterionRevision: review.CriterionRevision,
		Action: models.TaskCompletionControlReleaseHold, HoldKind: review.HoldKind,
		ActorKind: "system", ActorID: reviewerActor,
	}); err != nil {
		s.logger.Warn("completion gate review release failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

func (s *Service) recordReviewPending(ctx context.Context, taskID string, review models.TaskCompletionCriterion, reason string, detail []string) {
	links, err := s.prs.LinkedPRs(ctx, taskID)
	if err != nil || len(links) == 0 {
		return
	}
	if _, err := s.gates.RecordTaskCompletionEvaluation(ctx, models.TaskCompletionEvaluationChange{
		TaskID: taskID, CriterionID: review.ID, CriterionRevision: review.CriterionRevision,
		SubjectRevision: HeadsRevision(links), Result: models.TaskCompletionResultPending, Reason: reason,
		Detail: detail, VerifierKind: VerifierKindReviews, VerifierID: VerifierKindReviews,
	}); err != nil {
		s.logger.Warn("completion gate review wait record failed", zap.String("task_id", taskID), zap.Error(err))
	}
}
