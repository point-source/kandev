package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const hostGateTaskID = "host-gate-task"

func newHostGateRepo(t *testing.T) (*Repository, *sqlx.DB) {
	t.Helper()
	repo, writer, _, _ := newCompletionGateReadTestRepo(t)
	if _, err := writer.Exec(`CREATE TABLE IF NOT EXISTS github_task_prs (
		id TEXT PRIMARY KEY, task_id TEXT NOT NULL, head_sha TEXT NOT NULL,
		state TEXT NOT NULL DEFAULT 'open', detached_at TIMESTAMP)`); err != nil {
		t.Fatalf("create github_task_prs: %v", err)
	}
	return repo, writer
}

func ciCriterion() models.TaskCompletionCriterion {
	return models.TaskCompletionCriterion{
		ID: "ci", Description: "CI passes at the PR head", Kind: models.TaskCompletionCriterionCIAtHead,
		EvidenceSubject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHeads, ID: hostGateTaskID},
		Config:          models.TaskCompletionCriterionConfig{ProtectedPaths: []string{"tests/**"}},
		OwnerKind:       "agent", OwnerID: "ceo",
	}
}

func createHostGateTask(t *testing.T, repo *Repository, criteria ...models.TaskCompletionCriterion) {
	t.Helper()
	if err := repo.CreateTask(context.Background(), &models.Task{
		ID: hostGateTaskID, WorkspaceID: "ws-host", Title: "Gated", CompletionCriteria: criteria,
	}); err != nil {
		t.Fatalf("create task: %v", err)
	}
}

func linkPR(t *testing.T, writer *sqlx.DB, id, head string) {
	t.Helper()
	if _, err := writer.Exec(`INSERT INTO github_task_prs (id, task_id, head_sha) VALUES (?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET head_sha = excluded.head_sha`, id, hostGateTaskID, head); err != nil {
		t.Fatalf("link PR: %v", err)
	}
}

func gate(t *testing.T, repo *Repository) *models.TaskCompletionGateSnapshot {
	t.Helper()
	snapshot, err := repo.GetTaskCompletionGate(context.Background(), hostGateTaskID)
	if err != nil {
		t.Fatalf("read gate: %v", err)
	}
	return snapshot
}

func blockerReasons(snapshot *models.TaskCompletionGateSnapshot) []string {
	reasons := make([]string, 0, len(snapshot.Blockers))
	for _, blocker := range snapshot.Blockers {
		reasons = append(reasons, blocker.Reason)
	}
	return reasons
}

func evaluate(repo *Repository, revision, result string, mutate ...func(*models.TaskCompletionEvaluationChange)) (*models.TaskCompletionEvaluationOutcome, error) {
	change := models.TaskCompletionEvaluationChange{
		TaskID: hostGateTaskID, CriterionID: "ci", CriterionRevision: 1, SubjectRevision: revision,
		Result: result, VerifierKind: "kandev_ci", VerifierID: "kandev_ci",
	}
	for _, apply := range mutate {
		apply(&change)
	}
	return repo.RecordTaskCompletionEvaluation(context.Background(), change)
}

func TestHostCriterionIsStoredWithTaskCreation(t *testing.T) {
	repo, _ := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())

	snapshot := gate(t, repo)
	if len(snapshot.Criteria) != 1 {
		t.Fatalf("criteria = %d, want 1", len(snapshot.Criteria))
	}
	criterion := snapshot.Criteria[0]
	if criterion.Kind != models.TaskCompletionCriterionCIAtHead || criterion.Result != models.TaskCompletionResultPending ||
		criterion.Confidence != models.TaskCompletionConfidenceMachine || criterion.OwnerID != "ceo" ||
		len(criterion.Config.ProtectedPaths) != 1 {
		t.Fatalf("criterion = %+v", criterion)
	}
	if snapshot.WeakestConfidence != models.TaskCompletionConfidenceMachine {
		t.Fatalf("weakest confidence = %q", snapshot.WeakestConfidence)
	}
	if got := blockerReasons(snapshot); len(got) != 1 || got[0] != models.TaskCompletionBlockerPRMissing {
		t.Fatalf("blockers = %v, want pr_missing", got)
	}
}

func TestRefusedCriteriaRefuseTaskCreation(t *testing.T) {
	repo, _ := newHostGateRepo(t)
	bad := ciCriterion()
	bad.EvidenceSubject.ID = "another-task"
	err := repo.CreateTask(context.Background(), &models.Task{ID: hostGateTaskID, Title: "Gated", CompletionCriteria: []models.TaskCompletionCriterion{bad}})
	if err == nil {
		t.Fatal("create succeeded with an invalid host criterion")
	}
	if _, getErr := repo.GetTask(context.Background(), hostGateTaskID); getErr == nil {
		t.Fatal("task row exists after refused criteria")
	}
}

func TestHostEvaluationPassesOnlyAtLiveHeads(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	revision := models.TaskPRHeadsRevision([]models.TaskPRHead{{ID: "pr-1", HeadSHA: "head-1"}})

	if _, err := evaluate(repo, "pr-1@stale", models.TaskCompletionResultPass); !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("stale evaluation err = %v", err)
	}
	outcome, err := evaluate(repo, revision, models.TaskCompletionResultPass)
	if err != nil || !outcome.Changed {
		t.Fatalf("pass err=%v outcome=%+v", err, outcome)
	}
	if snapshot := gate(t, repo); snapshot.Blocked {
		t.Fatalf("gate blocked after pass: %v", blockerReasons(snapshot))
	}

	linkPR(t, writer, "pr-1", "head-2")
	if got := blockerReasons(gate(t, repo)); len(got) != 1 || got[0] != models.TaskCompletionBlockerEvidenceStale {
		t.Fatalf("blockers after push = %v", got)
	}
}

func TestDetachedAndMergedPRsChangeTheGate(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	linkPR(t, writer, "pr-2", "head-9")
	both := models.TaskPRHeadsRevision([]models.TaskPRHead{{ID: "pr-2", HeadSHA: "head-9"}, {ID: "pr-1", HeadSHA: "head-1"}})
	if both != "pr-1@head-1,pr-2@head-9" {
		t.Fatalf("revision = %q", both)
	}
	if _, err := evaluate(repo, both, models.TaskCompletionResultPass); err != nil {
		t.Fatalf("pass: %v", err)
	}
	if _, err := writer.Exec(`UPDATE github_task_prs SET detached_at = CURRENT_TIMESTAMP WHERE id = 'pr-2'`); err != nil {
		t.Fatal(err)
	}
	if got := blockerReasons(gate(t, repo)); len(got) != 1 || got[0] != models.TaskCompletionBlockerEvidenceStale {
		t.Fatalf("blockers after detach = %v", got)
	}
	if _, err := writer.Exec(`UPDATE github_task_prs SET state = 'merged' WHERE id = 'pr-1'`); err != nil {
		t.Fatal(err)
	}
	if got := blockerReasons(gate(t, repo)); len(got) != 1 || got[0] != models.TaskCompletionBlockerPRMergedUnverified {
		t.Fatalf("blockers after merge = %v", got)
	}
}

func TestRepeatedEvaluationCountsOnce(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	revision := "pr-1@head-1"
	fail := func(change *models.TaskCompletionEvaluationChange) {
		change.Reason = "checks_failed"
		change.Detail = []string{"lint"}
		change.CountRework = true
	}
	first, err := evaluate(repo, revision, models.TaskCompletionResultFail, fail)
	if err != nil || !first.Changed || first.Criterion.ReworkCount != 1 {
		t.Fatalf("first fail err=%v outcome=%+v", err, first)
	}
	second, err := evaluate(repo, revision, models.TaskCompletionResultFail, fail)
	if err != nil || second.Changed || second.Criterion.ReworkCount != 1 {
		t.Fatalf("repeat fail err=%v outcome=%+v", err, second)
	}
	if detail := second.Criterion.ResultDetail; len(detail) != 1 || detail[0] != "lint" {
		t.Fatalf("detail = %v", detail)
	}
}

func TestNonPassResultClearsVerification(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPass); err != nil {
		t.Fatal(err)
	}
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultFail, func(c *models.TaskCompletionEvaluationChange) { c.Reason = "checks_failed" }); err != nil {
		t.Fatal(err)
	}
	snapshot := gate(t, repo)
	if !snapshot.Blocked || snapshot.Criteria[0].Evidence != nil {
		t.Fatalf("gate after fail = %+v", snapshot)
	}
}

func TestHoldBlocksUntilReleased(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultFail, func(c *models.TaskCompletionEvaluationChange) {
		c.Reason = "checks_failed"
		c.Hold = "rework_cap_reached"
	}); err != nil {
		t.Fatal(err)
	}
	snapshot := gate(t, repo)
	if got := blockerReasons(snapshot); len(got) != 1 || got[0] != models.TaskCompletionBlockerHeld {
		t.Fatalf("blockers = %v", got)
	}
	if snapshot.Criteria[0].HeldReason != "rework_cap_reached" || snapshot.Criteria[0].HeldAt == nil {
		t.Fatalf("hold = %+v", snapshot.Criteria[0])
	}
	released, err := repo.ApplyTaskCompletionControl(context.Background(), models.TaskCompletionControlChange{
		TaskID: hostGateTaskID, CriterionID: "ci", CriterionRevision: 1,
		Action: models.TaskCompletionControlExtendCaps, MaxReworkRounds: 5,
		ActorKind: "human", ActorID: "user-1", Reason: "one more try",
	})
	if err != nil {
		t.Fatalf("extend caps: %v", err)
	}
	criterion := released.Snapshot.Criteria[0]
	if criterion.HeldReason != "" || criterion.Config.MaxReworkRounds != 5 {
		t.Fatalf("after extend = %+v", criterion)
	}
}

func TestAllowProtectedPathsBindsToHeads(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPending, func(c *models.TaskCompletionEvaluationChange) {
		c.Hold = "protected_path_changed"
		c.Detail = []string{"tests/a_test.go"}
	}); err != nil {
		t.Fatal(err)
	}
	allow := models.TaskCompletionControlChange{
		TaskID: hostGateTaskID, CriterionID: "ci", CriterionRevision: 1,
		Action: models.TaskCompletionControlAllowProtected, SubjectRevision: "pr-1@old",
		ActorKind: "human", ActorID: "user-1",
	}
	if _, err := repo.ApplyTaskCompletionControl(context.Background(), allow); !errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		t.Fatalf("stale allow err = %v", err)
	}
	allow.SubjectRevision = "pr-1@head-1"
	snapshot, err := repo.ApplyTaskCompletionControl(context.Background(), allow)
	if err != nil {
		t.Fatalf("allow: %v", err)
	}
	criterion := snapshot.Snapshot.Criteria[0]
	if criterion.HeldReason != "" || criterion.ProtectedAllowedRevision != "pr-1@head-1" || criterion.ProtectedAllowedBy != "user-1" {
		t.Fatalf("after allow = %+v", criterion)
	}
}

func TestExistingVerifyPathRefusesHostCriteria(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	snapshot := gate(t, repo)
	_, err := repo.VerifyTaskCompletionCriterion(context.Background(), models.TaskCompletionEvidenceChange{
		TaskID: hostGateTaskID, WorkspaceID: snapshot.WorkspaceID, ExpectedRevision: snapshot.Revision, CriterionID: "ci",
		Evidence:  models.TaskCompletionEvidence{Subject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHeads, ID: hostGateTaskID, Revision: "pr-1@head-1"}},
		ActorKind: "human", ActorID: "default",
	})
	if !errors.Is(err, repoerrors.ErrTaskCompletionVerifierReserved) {
		t.Fatalf("verify err = %v, want verifier reserved", err)
	}
}

func TestCriterionRetentionKeepsStateAndTakesNewCaps(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultFail, func(c *models.TaskCompletionEvaluationChange) {
		c.Reason = "checks_failed"
		c.CountRework = true
	}); err != nil {
		t.Fatal(err)
	}
	current := gate(t, repo)
	retained := ciCriterion()
	retained.Config.MaxReworkRounds = 7
	snapshot, err := repo.SetTaskCompletionCriteria(context.Background(), models.TaskCompletionCriteriaChange{
		TaskID: hostGateTaskID, WorkspaceID: current.WorkspaceID, ExpectedRevision: current.Revision,
		Criteria: []models.TaskCompletionCriterion{retained}, ActorKind: "human", ActorID: "user-1",
	})
	if err != nil {
		t.Fatalf("retain: %v", err)
	}
	criterion := snapshot.Criteria[0]
	if criterion.CriterionRevision != 1 || criterion.ReworkCount != 1 || criterion.Config.MaxReworkRounds != 7 {
		t.Fatalf("retained criterion = %+v", criterion)
	}

	changed := ciCriterion()
	changed.Config.ProtectedPaths = []string{"tests/**", ".github/**"}
	snapshot, err = repo.SetTaskCompletionCriteria(context.Background(), models.TaskCompletionCriteriaChange{
		TaskID: hostGateTaskID, WorkspaceID: current.WorkspaceID, ExpectedRevision: snapshot.Revision,
		Criteria: []models.TaskCompletionCriterion{changed}, ActorKind: "human", ActorID: "user-1",
		HumanConfirmationRevision: snapshot.Revision, HumanConfirmationReason: "stronger",
	})
	if err != nil {
		t.Fatalf("change: %v", err)
	}
	criterion = snapshot.Criteria[0]
	if criterion.CriterionRevision != 2 || criterion.ReworkCount != 0 || criterion.Result != models.TaskCompletionResultPending {
		t.Fatalf("changed criterion = %+v", criterion)
	}
}

func TestBatchSummaryCountsHostCriteria(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPass); err != nil {
		t.Fatal(err)
	}
	batch, err := repo.GetTaskCompletionGateSummaries(context.Background(), []string{hostGateTaskID})
	if err != nil {
		t.Fatal(err)
	}
	observation := batch.ByTaskID[hostGateTaskID]
	if observation == nil || observation.VerifiedCount != 1 || observation.Blocked ||
		observation.WeakestConfidence != models.TaskCompletionConfidenceMachine {
		t.Fatalf("observation = %+v", observation)
	}
	linkPR(t, writer, "pr-1", "head-2")
	batch, err = repo.GetTaskCompletionGateSummaries(context.Background(), []string{hostGateTaskID})
	if err != nil {
		t.Fatal(err)
	}
	if observation := batch.ByTaskID[hostGateTaskID]; observation == nil || !observation.Blocked {
		t.Fatalf("observation after push = %+v", observation)
	}
}

func TestHostCriterionColumnsSurviveSchemaReplay(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	replayed, err := NewWithDB(writer, writer, nil)
	if err != nil {
		t.Fatalf("replay schema: %v", err)
	}
	snapshot := gate(t, replayed)
	if len(snapshot.Criteria) != 1 || snapshot.Criteria[0].Kind != models.TaskCompletionCriterionCIAtHead ||
		len(snapshot.Criteria[0].Config.ProtectedPaths) != 1 {
		t.Fatalf("criterion after replay = %+v", snapshot.Criteria)
	}
}

func TestEvaluationReleasesOnlyTheNamedHold(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPending, func(c *models.TaskCompletionEvaluationChange) {
		c.Hold = "protected_path_changed"
	}); err != nil {
		t.Fatal(err)
	}
	linkPR(t, writer, "pr-1", "head-2")
	outcome, err := evaluate(repo, "pr-1@head-2", models.TaskCompletionResultPass, func(c *models.TaskCompletionEvaluationChange) {
		c.ReleaseHold = "rework_cap_reached"
	})
	if err != nil || outcome.Criterion.HeldReason != "protected_path_changed" {
		t.Fatalf("mismatched release err=%v criterion=%+v", err, outcome.Criterion)
	}
	outcome, err = evaluate(repo, "pr-1@head-2", models.TaskCompletionResultPass, func(c *models.TaskCompletionEvaluationChange) {
		c.ReleaseHold = "protected_path_changed"
	})
	if err != nil || !outcome.Changed || outcome.Criterion.HeldReason != "" || outcome.Snapshot.Blocked {
		t.Fatalf("release err=%v outcome=%+v", err, outcome)
	}
}

func TestListTaskIDsWithUnmetCompletionCriteria(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	ids, err := repo.ListTaskIDsWithUnmetCompletionCriteria(context.Background(), models.TaskCompletionSweepQuery{Kind: models.TaskCompletionCriterionCIAtHead, Limit: 10})
	if err != nil || len(ids) != 1 || ids[0] != hostGateTaskID {
		t.Fatalf("unmet ids = %v, %v", ids, err)
	}
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPass); err != nil {
		t.Fatal(err)
	}
	ids, err = repo.ListTaskIDsWithUnmetCompletionCriteria(context.Background(), models.TaskCompletionSweepQuery{Kind: models.TaskCompletionCriterionCIAtHead, Limit: 10})
	if err != nil || len(ids) != 0 {
		t.Fatalf("unmet ids after pass = %v, %v", ids, err)
	}
}

func TestCountReworkIsKeyedAndCapped(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	count := func(key string) models.TaskCompletionCriterion {
		t.Helper()
		snapshot, err := repo.ApplyTaskCompletionControl(context.Background(), models.TaskCompletionControlChange{
			TaskID: hostGateTaskID, CriterionID: "ci", CriterionRevision: 1,
			Action: models.TaskCompletionControlCountRework, Key: key, ReworkCap: 2,
			ActorKind: "system", ActorID: "router",
		})
		if err != nil {
			t.Fatalf("count %s: %v", key, err)
		}
		return snapshot.Snapshot.Criteria[0]
	}
	if c := count("a"); c.ReworkCount != 1 {
		t.Fatalf("first count = %+v", c)
	}
	if c := count("a"); c.ReworkCount != 1 {
		t.Fatalf("repeated key counted: %+v", c)
	}
	if c := count("b"); c.ReworkCount != 2 || c.HeldReason != "" {
		t.Fatalf("second count = %+v", c)
	}
	if c := count("c"); c.ReworkCount != 2 || c.HeldReason != "rework_cap_reached" {
		t.Fatalf("over cap = %+v", c)
	}
}

func TestListHeldCompletionCriteria(t *testing.T) {
	repo, writer := newHostGateRepo(t)
	createHostGateTask(t, repo, ciCriterion())
	linkPR(t, writer, "pr-1", "head-1")
	if held, err := repo.ListHeldCompletionCriteria(context.Background(), "ws-host"); err != nil || len(held) != 0 {
		t.Fatalf("held before = %v, %v", held, err)
	}
	if _, err := evaluate(repo, "pr-1@head-1", models.TaskCompletionResultPending, func(c *models.TaskCompletionEvaluationChange) {
		c.Hold = "protected_path_changed"
	}); err != nil {
		t.Fatal(err)
	}
	held, err := repo.ListHeldCompletionCriteria(context.Background(), "ws-host")
	if err != nil || len(held) != 1 || held[0].TaskTitle != "Gated" || held[0].Criterion.HeldReason != "protected_path_changed" {
		t.Fatalf("held = %+v, %v", held, err)
	}
}
