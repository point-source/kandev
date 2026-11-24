package sqlite

import (
	"fmt"
	"strings"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// applyPersonControl applies a person's Allow, Send back, Accept or Approve.
func applyPersonControl(next *models.TaskCompletionCriterion, criterion models.TaskCompletionCriterion, change models.TaskCompletionControlChange, now time.Time) error {
	switch change.Action {
	case models.TaskCompletionControlAllowProtected:
		if err := requireProtectedHold(criterion, change); err != nil {
			return err
		}
		next.ProtectedAllowedRevision, next.ProtectedAllowedBy = change.SubjectRevision, change.ActorID
		next.HoldKind, next.HeldAt, next.EscalatedAt = "", nil, nil
	case models.TaskCompletionControlSendBack:
		if err := requireSendBack(criterion, change); err != nil {
			return err
		}
		applyPersonResult(next, change, models.TaskCompletionResultFail, models.TaskCompletionReasonSentBack, now)
	case models.TaskCompletionControlApprove:
		if err := requireApprove(criterion, change); err != nil {
			return err
		}
		applyPersonResult(next, change, models.TaskCompletionResultPass, models.TaskCompletionReasonApproved, now)
	default:
		if err := requireAccept(criterion, change); err != nil {
			return err
		}
		applyPersonResult(next, change, models.TaskCompletionResultPass, models.TaskCompletionReasonAccepted, now)
	}
	return nil
}

// applyPersonResult records a person's result at the observed heads and
// releases any hold. Only a pass keeps verification, attributed to the person.
func applyPersonResult(next *models.TaskCompletionCriterion, change models.TaskCompletionControlChange, result, reason string, now time.Time) {
	if next.ResultSubjectRevision != change.SubjectRevision || next.ResultFirstSeenAt == nil {
		next.ResultFirstSeenAt = &now
	}
	next.Result, next.ResultReason, next.ResultSubjectRevision, next.ResultAt = result, reason, change.SubjectRevision, &now
	next.ResultDetail = boundedCompletionDetail([]string{strings.TrimSpace(change.Reason)})
	next.HoldKind, next.HeldAt, next.EscalatedAt = "", nil, nil
	applyEvaluationVerification(next, models.TaskCompletionEvaluationChange{
		Result: result, SubjectRevision: change.SubjectRevision,
		VerifierKind: models.TaskCompletionVerifierHuman, VerifierID: change.ActorID,
	}, now)
}

// requireProtectedHold accepts Allow only for a protected-path hold
// evaluated at the heads the person saw.
func requireProtectedHold(criterion models.TaskCompletionCriterion, change models.TaskCompletionControlChange) error {
	if criterion.Kind != models.TaskCompletionCriterionCheck || criterion.HoldKind != holdProtectedFilesChanged ||
		criterion.ResultSubjectRevision != change.SubjectRevision {
		return repoerrors.ErrTaskCompletionEvidenceChanged
	}
	return nil
}

// requireSendBack accepts a send back of a check only from a protected-path
// hold, and of a review at any time. Both need a reason.
func requireSendBack(criterion models.TaskCompletionCriterion, change models.TaskCompletionControlChange) error {
	if criterion.Kind == models.TaskCompletionCriterionCheck {
		if err := requireProtectedHold(criterion, change); err != nil {
			return err
		}
	}
	if strings.TrimSpace(change.Reason) == "" {
		return fmt.Errorf("a reason is required to send the task back")
	}
	return nil
}

// requireAccept accepts only a held review, with a reason. A review that is
// no longer held changed since the person saw it.
func requireAccept(criterion models.TaskCompletionCriterion, change models.TaskCompletionControlChange) error {
	if criterion.Kind != models.TaskCompletionCriterionReview || criterion.HoldKind == "" {
		return repoerrors.ErrTaskCompletionEvidenceChanged
	}
	if strings.TrimSpace(change.Reason) == "" {
		return fmt.Errorf("a reason is required to accept a review")
	}
	return nil
}

// requireApprove accepts only an approval that asks a person at the heads
// they saw. Anything else changed since the person saw it.
func requireApprove(criterion models.TaskCompletionCriterion, change models.TaskCompletionControlChange) error {
	if criterion.Kind != models.TaskCompletionCriterionApproval || criterion.HoldKind != "" ||
		criterion.ResultReason != models.TaskCompletionReasonAwaitingPerson || criterion.ResultSubjectRevision != change.SubjectRevision {
		return repoerrors.ErrTaskCompletionEvidenceChanged
	}
	return nil
}
