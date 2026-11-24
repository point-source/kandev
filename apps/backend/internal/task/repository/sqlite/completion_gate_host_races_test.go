package sqlite

import (
	"context"
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// A verifier that read its inputs before a person's verdict must not replace
// that verdict at the same heads.
//
// @covers AC-OFFICE-COMPLETION-GATES-004.6
func TestALateVerifierResultLeavesAPersonsVerdictAtTheSameHeads(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, reviewHostCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := control(repo, models.TaskCompletionControlSendBack, onReview, func(c *models.TaskCompletionControlChange) {
		c.SubjectRevision, c.Reason = "pr-1@head-1", "missed the error path"
	}); err != nil {
		t.Fatal(err)
	}
	outcome, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPass, func(c *models.TaskCompletionEvaluationChange) {
		c.CriterionID, c.VerifierKind, c.VerifierID = "review", "review_seats", "qa"
	})
	if err != nil || outcome.Changed {
		t.Fatalf("late pass changed=%v err=%v", outcome != nil && outcome.Changed, err)
	}
	if review := gate(t, repo).Criteria[0]; review.ResultReason != models.TaskCompletionReasonSentBack {
		t.Fatalf("review after a late pass = %+v", review)
	}
	linkPR(t, writer, "pr-1", "head-2")
	if outcome, err := evaluate(repo, "pr-1@head-2", models.TaskCompletionResultPass, func(c *models.TaskCompletionEvaluationChange) {
		c.CriterionID, c.VerifierKind, c.VerifierID = "review", "review_seats", "qa"
	}); err != nil || !outcome.Changed {
		t.Fatalf("pass at new heads changed=%v err=%v", outcome != nil && outcome.Changed, err)
	}
}

// A hold conditioned on the result reason it observed is not applied once
// that reason changed, so a wait that ended cannot be held afterwards.
//
// @covers AC-OFFICE-COMPLETION-GATES-009.3
func TestAHoldConditionedOnAnEndedWaitIsNotApplied(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, reviewHostCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	waitHold := func() (*models.TaskCompletionControlOutcome, error) {
		return control(repo, models.TaskCompletionControlHold, onReview, func(c *models.TaskCompletionControlChange) {
			c.HoldKind, c.Detail, c.ActorKind, c.ActorID = "review_wait_limit_reached", []string{"since"}, "system", "completion_gates"
			c.RequireResultReason = "waiting_for_reviewer"
		})
	}
	if outcome, err := waitHold(); err != nil || outcome.Claimed {
		t.Fatalf("hold of a review that is not waiting claimed=%v err=%v", outcome != nil && outcome.Claimed, err)
	}
	if review := gate(t, repo).Criteria[0]; review.HoldKind != "" {
		t.Fatalf("review = %+v", review)
	}
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPending, func(c *models.TaskCompletionEvaluationChange) {
		c.CriterionID, c.Reason, c.VerifierKind, c.VerifierID = "review", "waiting_for_reviewer", "review_seats", "review_seats"
	}); err != nil {
		t.Fatal(err)
	}
	if outcome, err := waitHold(); err != nil || !outcome.Claimed {
		t.Fatalf("hold of a waiting review claimed=%v err=%v", outcome != nil && outcome.Claimed, err)
	}
}

// The heads of several linked PRs form a revision longer than a scalar
// evidence revision; a pass recorded at those heads still satisfies the gate.
//
// @covers AC-OFFICE-COMPLETION-GATES-002.1
func TestAPassAtTheHeadsOfFourLinkedPRsSatisfiesTheGate(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	for _, id := range []string{"0", "1", "2", "3"} {
		linkPR(t, writer, strings.Repeat(id, 36), strings.Repeat("a", 40))
	}
	heads := gate(t, repo).HeadsRevision
	if len(heads) <= 256 {
		t.Fatalf("heads revision is only %d characters", len(heads))
	}
	if _, err := evaluate(repo, heads, models.TaskCompletionResultPass); err != nil {
		t.Fatal(err)
	}
	if snapshot := gate(t, repo); snapshot.Blocked {
		t.Fatalf("blockers = %v", blockerReasons(snapshot))
	}
}

// Kandev verifies one check and one review per task, so a second criterion of
// either kind would never be verified.
//
// @covers AC-OFFICE-COMPLETION-GATES-001.1
func TestATaskCarriesAtMostOneCriterionOfEachHostKind(t *testing.T) {
	for name, second := range map[string]models.TaskCompletionCriterion{"check": ciCriterion(), "review": reviewHostCriterion()} {
		repo, _ := newHostGateRepo(t)
		first := second
		second.ID = "second"
		err := repo.CreateTask(context.Background(), &models.Task{
			ID: hostGateTaskID, WorkspaceID: "ws-host", Title: "Gated", CompletionCriteria: []models.TaskCompletionCriterion{first, second},
		})
		if err == nil {
			t.Errorf("%s: two criteria of one kind were stored", name)
		}
	}
}
