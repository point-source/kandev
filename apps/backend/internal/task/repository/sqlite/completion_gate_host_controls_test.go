package sqlite

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

func control(repo *Repository, action string, mutate ...func(*models.TaskCompletionControlChange)) (*models.TaskCompletionControlOutcome, error) {
	change := models.TaskCompletionControlChange{
		TaskID: hostGateTaskID, CriterionID: "ci", CriterionRevision: 1, Action: action,
		ActorKind: "human", ActorID: "user-1",
	}
	for _, apply := range mutate {
		apply(&change)
	}
	return repo.ApplyTaskCompletionControl(context.Background(), change)
}

func TestFirstPassAfterMergeStaysUnverified(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := writer.Exec(`UPDATE github_task_prs SET state = 'merged' WHERE id = 'pr-1'`); err != nil {
		t.Fatal(err)
	}
	outcome, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPass)
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Criterion.Result != models.TaskCompletionResultPending || outcome.Criterion.Evidence != nil ||
		outcome.Criterion.ResultReason != models.TaskCompletionBlockerPRMergedUnverified {
		t.Fatalf("pass after merge = %+v", outcome.Criterion)
	}
}

func TestPassRecordedBeforeMergeSurvivesReevaluation(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPass); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Exec(`UPDATE github_task_prs SET state = 'merged' WHERE id = 'pr-1'`); err != nil {
		t.Fatal(err)
	}
	outcome, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPass)
	if err != nil || outcome.Criterion.Result != models.TaskCompletionResultPass || outcome.Criterion.Evidence == nil {
		t.Fatalf("re-evaluated pass = %+v, %v", outcome, err)
	}
}

func holdProtected(t *testing.T, repo *Repository) {
	t.Helper()
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPending, func(c *models.TaskCompletionEvaluationChange) {
		c.Hold, c.Reason = "protected_files_changed", "protected_files_changed"
	}); err != nil {
		t.Fatal(err)
	}
}

func TestProtectedControlsRequireAProtectedHoldAtTheSeenHeads(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	atHead := func(c *models.TaskCompletionControlChange) {
		c.SubjectRevision = "pr-1@head-1"
		c.Reason = "weakened tests"
	}
	if _, err := control(repo, models.TaskCompletionControlAllowProtected, atHead); !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("allow without a hold err = %v", err)
	}
	holdProtected(t, repo)
	if _, err := control(repo, models.TaskCompletionControlSendBack, func(c *models.TaskCompletionControlChange) {
		c.SubjectRevision = "pr-1@head-1"
	}); err == nil {
		t.Fatal("send back without a reason was accepted")
	}
	outcome, err := control(repo, models.TaskCompletionControlSendBack, atHead)
	if err != nil {
		t.Fatal(err)
	}
	criterion := outcome.Snapshot.Criteria[0]
	if criterion.HoldKind != "" || criterion.ProtectedAllowedRevision != "" || criterion.Result != models.TaskCompletionResultFail ||
		criterion.ResultReason != models.TaskCompletionReasonSentBack || len(criterion.ResultDetail) != 1 ||
		criterion.ResultDetail[0] != "weakened tests" || criterion.ResultSubjectRevision != "pr-1@head-1" {
		t.Fatalf("after send back = %+v", criterion)
	}
	if _, err := control(repo, models.TaskCompletionControlSendBack, atHead); !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("second send back err = %v", err)
	}
}

func TestReworkCountReportsTheClaimAndRemembersRecentKeys(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	count := func(key string) *models.TaskCompletionControlOutcome {
		t.Helper()
		outcome, err := control(repo, models.TaskCompletionControlCountRework, func(c *models.TaskCompletionControlChange) {
			c.Key, c.CountReworkLimit, c.ActorKind = key, 10, "system"
		})
		if err != nil {
			t.Fatal(err)
		}
		return outcome
	}
	if !count("a").Claimed || !count("b").Claimed {
		t.Fatal("new keys were not claimed")
	}
	if outcome := count("a"); outcome.Claimed || outcome.Snapshot.Criteria[0].ReworkCount != 2 {
		t.Fatalf("redelivered older key = %+v", outcome.Snapshot.Criteria[0])
	}
}

func TestCapExtensionReleasesOnlyTheCausingCapAboveUse(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	for _, key := range []string{"a", "b"} {
		if _, err := control(repo, models.TaskCompletionControlCountRework, func(c *models.TaskCompletionControlChange) {
			c.Key, c.CountReworkLimit = key, 2
		}); err != nil {
			t.Fatal(err)
		}
	}
	held, err := control(repo, models.TaskCompletionControlCountRework, func(c *models.TaskCompletionControlChange) { c.Key, c.CountReworkLimit = "c", 2 })
	if err != nil || held.Snapshot.Criteria[0].HoldKind != "rework_limit_reached" {
		t.Fatalf("hold = %+v, %v", held, err)
	}
	extend := func(attempts, rework int) models.TaskCompletionCriterion {
		t.Helper()
		outcome, err := control(repo, models.TaskCompletionControlExtendLimits, func(c *models.TaskCompletionControlChange) {
			c.VerificationLimit, c.ReworkLimit = attempts, rework
		})
		if err != nil {
			t.Fatal(err)
		}
		return outcome.Snapshot.Criteria[0]
	}
	if c := extend(9, 0); c.HoldKind != "rework_limit_reached" {
		t.Fatalf("raising the attempt limit released a rework hold: %+v", c)
	}
	if c := extend(0, 2); c.HoldKind != "rework_limit_reached" {
		t.Fatalf("a rework limit equal to use released the hold: %+v", c)
	}
	if c := extend(0, 3); c.HoldKind != "" {
		t.Fatalf("raising the rework limit above use kept the hold: %+v", c)
	}
}

func TestStateRevisionAndEscalationMarker(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	before := gate(t, repo).StateRevision
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultFail, func(c *models.TaskCompletionEvaluationChange) {
		c.Reason, c.Hold = "checks_failed", "rework_limit_reached"
	}); err != nil {
		t.Fatal(err)
	}
	afterResult := gate(t, repo).StateRevision
	if afterResult <= before {
		t.Fatalf("state revision did not advance on a result: %d -> %d", before, afterResult)
	}
	query := models.TaskCompletionSweepQuery{HeldUnescalated: true, Limit: 10}
	if ids, err := repo.ListTaskIDsWithUnmetCompletionCriteria(context.Background(), query); err != nil || len(ids) != 1 {
		t.Fatalf("unescalated holds = %v, %v", ids, err)
	}
	outcome, err := control(repo, models.TaskCompletionControlMarkEscalated, func(c *models.TaskCompletionControlChange) { c.ActorKind = "system" })
	if err != nil || outcome.Snapshot.Criteria[0].EscalatedAt == nil || outcome.Snapshot.StateRevision <= afterResult {
		t.Fatalf("mark escalated = %+v, %v", outcome, err)
	}
	if ids, err := repo.ListTaskIDsWithUnmetCompletionCriteria(context.Background(), query); err != nil || len(ids) != 0 {
		t.Fatalf("escalated hold still listed: %v, %v", ids, err)
	}
}

func TestDueSweepSelectsNeverEvaluatedAndExpiredAwaitingCriteria(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	due := func(at time.Time) []string {
		t.Helper()
		ids, err := repo.ListTaskIDsWithUnmetCompletionCriteria(context.Background(), models.TaskCompletionSweepQuery{
			Kind: models.TaskCompletionCriterionCheck, DueBefore: &at, AwaitingReason: "awaiting_checks", Limit: 10,
		})
		if err != nil {
			t.Fatal(err)
		}
		return ids
	}
	now := time.Now().UTC()
	if ids := due(now.Add(-time.Hour)); len(ids) != 1 {
		t.Fatalf("never-evaluated criterion not due: %v", ids)
	}
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPending, func(c *models.TaskCompletionEvaluationChange) {
		c.Reason = "awaiting_checks"
	}); err != nil {
		t.Fatal(err)
	}
	if ids := due(now.Add(-time.Hour)); len(ids) != 0 {
		t.Fatalf("criterion inside its grace was due: %v", ids)
	}
	if ids := due(now.Add(time.Hour)); len(ids) != 1 {
		t.Fatalf("criterion past its grace was not due: %v", ids)
	}
}
