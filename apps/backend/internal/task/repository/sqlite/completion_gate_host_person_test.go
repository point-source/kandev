package sqlite

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func reviewHostCriterion() models.TaskCompletionCriterion {
	criterion := ciCriterion()
	criterion.ID, criterion.Kind, criterion.Config = "review", models.TaskCompletionCriterionReview, models.TaskCompletionCriterionConfig{}
	return criterion
}

func onReview(change *models.TaskCompletionControlChange) { change.CriterionID = "review" }

func holdReview(t *testing.T, repo *Repository, kind string, detail ...string) {
	t.Helper()
	if _, err := control(repo, models.TaskCompletionControlHold, onReview, func(c *models.TaskCompletionControlChange) {
		c.HoldKind, c.Detail, c.ActorKind, c.ActorID = kind, detail, "system", "completion_gates"
		c.SubjectRevision = "pr-1@head-1"
	}); err != nil {
		t.Fatal(err)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-007.7
func TestHoldControlRecordsTheHoldKindOnce(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, reviewHostCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	holdReview(t, repo, "no_reviewer_available", "assignee_only")
	if heads := gate(t, repo).HeadsRevision; heads != "pr-1@head-1" {
		t.Fatalf("snapshot heads = %q", heads)
	}
	review := gate(t, repo).Criteria[0]
	if review.HoldKind != "no_reviewer_available" || review.HeldAt == nil || len(review.ResultDetail) != 1 ||
		review.ResultDetail[0] != "assignee_only" || review.ResultSubjectRevision != "pr-1@head-1" {
		t.Fatalf("held review = %+v", review)
	}
	again, err := control(repo, models.TaskCompletionControlHold, onReview, func(c *models.TaskCompletionControlChange) {
		c.HoldKind, c.Detail, c.ActorKind, c.ActorID = "no_reviewer_available", []string{"assignee_only"}, "system", "completion_gates"
	})
	if err != nil || again.Claimed {
		t.Fatalf("repeated hold claimed=%v err=%v", again != nil && again.Claimed, err)
	}
	if _, err := control(repo, models.TaskCompletionControlReleaseHold, onReview, func(c *models.TaskCompletionControlChange) {
		c.HoldKind, c.ActorKind, c.ActorID = "review_wait_limit_reached", "system", "completion_gates"
	}); err != nil {
		t.Fatal(err)
	}
	if gate(t, repo).Criteria[0].HoldKind != "no_reviewer_available" {
		t.Fatal("release of another hold kind cleared the hold")
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-008.9
func TestAcceptVerifiesAHeldReviewAtTheHeadsShown(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion(), reviewHostCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	accept := func(subject, reason string) error {
		_, err := control(repo, models.TaskCompletionControlAccept, onReview, func(c *models.TaskCompletionControlChange) {
			c.SubjectRevision, c.Reason = subject, reason
		})
		return err
	}
	if err := accept("pr-1@head-1", "looked at it"); err == nil {
		t.Fatal("accepted a review that is not held")
	}
	holdReview(t, repo, "rework_limit_reached")
	if err := accept("pr-1@head-1", " "); err == nil {
		t.Fatal("accepted without a reason")
	}
	if err := accept("pr-1@head-0", "looked at it"); !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("stale heads err = %v", err)
	}
	if err := accept("pr-1@head-1", "looked at it"); err != nil {
		t.Fatal(err)
	}
	snapshot := gate(t, repo)
	review := snapshot.Criteria[1]
	if review.Result != models.TaskCompletionResultPass || review.HoldKind != "" || review.VerifierID != "user-1" ||
		review.VerdictSource != models.TaskCompletionVerdictPerson || review.Evidence == nil || review.Evidence.Subject.Revision != "pr-1@head-1" {
		t.Fatalf("accepted review = %+v", review)
	}
	if !historyHasVerdict(t, repo, models.TaskCompletionControlAccept, models.TaskCompletionVerdictPerson) {
		t.Fatal("history does not record the acceptance as a person's verdict")
	}
	if reasons := blockerReasons(snapshot); len(reasons) != 1 {
		t.Fatalf("blockers = %v, want only the unmet check", reasons)
	}
	if _, err := control(repo, models.TaskCompletionControlAccept, func(c *models.TaskCompletionControlChange) {
		c.SubjectRevision, c.Reason = "pr-1@head-1", "not a review"
	}); err == nil {
		t.Fatal("accepted a check criterion")
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-004.6
func TestSendBackFailsAReviewAtTheHeadsShown(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, reviewHostCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPass, func(c *models.TaskCompletionEvaluationChange) {
		c.CriterionID, c.VerifierKind, c.VerifierID = "review", "review_seats", "qa"
	}); err != nil {
		t.Fatal(err)
	}
	sendBack := func(subject string) error {
		_, err := control(repo, models.TaskCompletionControlSendBack, onReview, func(c *models.TaskCompletionControlChange) {
			c.SubjectRevision, c.Reason = subject, "missed the error path"
		})
		return err
	}
	if err := sendBack("pr-1@head-0"); !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("stale heads err = %v", err)
	}
	if err := sendBack("pr-1@head-1"); err != nil {
		t.Fatal(err)
	}
	review := gate(t, repo).Criteria[0]
	if review.Result != models.TaskCompletionResultFail || review.ResultReason != models.TaskCompletionReasonSentBack ||
		review.ResultSubjectRevision != "pr-1@head-1" || review.Evidence != nil || review.VerdictSource != "" ||
		len(review.ResultDetail) != 1 || review.ResultDetail[0] != "missed the error path" {
		t.Fatalf("sent back review = %+v", review)
	}
}

func historyHasVerdict(t *testing.T, repo *Repository, action, source string) bool {
	t.Helper()
	history, err := repo.ListTaskCompletionGateHistory(context.Background(), hostGateTaskID)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range history {
		if entry.Action == action && strings.Contains(entry.Details, `"verdict_source":"`+source+`"`) && entry.Reason != "" {
			return true
		}
	}
	return false
}

// @covers AC-OFFICE-COMPLETION-GATES-009.4
func TestSweepSelectsReviewsHeldWithAHoldKind(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, reviewHostCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	query := models.TaskCompletionSweepQuery{Kind: models.TaskCompletionCriterionReview, HoldKind: "no_reviewer_available", Limit: 10}
	if ids, err := repo.ListTaskIDsWithUnmetCompletionCriteria(context.Background(), query); err != nil || len(ids) != 0 {
		t.Fatalf("unheld ids = %v, %v", ids, err)
	}
	holdReview(t, repo, "no_reviewer_available", "assignee_only")
	if ids, err := repo.ListTaskIDsWithUnmetCompletionCriteria(context.Background(), query); err != nil || len(ids) != 1 || ids[0] != hostGateTaskID {
		t.Fatalf("held ids = %v, %v", ids, err)
	}
	query.HoldKind = "review_wait_limit_reached"
	if ids, err := repo.ListTaskIDsWithUnmetCompletionCriteria(context.Background(), query); err != nil || len(ids) != 0 {
		t.Fatalf("other hold kind ids = %v, %v", ids, err)
	}
}
