package service

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// @covers AC-TASKS-COMPLETION-006.9
func TestCreateTaskStoresHostCriteriaForItsOwnPRHeads(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := authn.WithIdentity(t.Context(), authn.Identity{UserID: "user-1", Role: authn.RoleAdmin})
	wf := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "gate-create")
	ensureTaskPRTable(t, repo)
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "gate-create", WorkflowID: wf, Title: "Gated",
		CompletionCriteria: []models.TaskCompletionCriterion{{
			ID: "ci", Kind: models.TaskCompletionCriterionCIAtHead,
			Config: models.TaskCompletionCriterionConfig{ProtectedPaths: []string{"tests/**"}},
			// Caller-supplied state is discarded.
			ReworkCount: 9, HeldReason: "forged", Result: models.TaskCompletionResultPass,
		}},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	gate, err := svc.GetTaskCompletionGate(ctx, created.Task.ID)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	if len(gate.Criteria) != 1 {
		t.Fatalf("criteria = %d", len(gate.Criteria))
	}
	criterion := gate.Criteria[0]
	if criterion.EvidenceSubject.Kind != models.TaskCompletionEvidenceGitHubPRHeads || criterion.EvidenceSubject.ID != created.Task.ID {
		t.Fatalf("subject = %+v", criterion.EvidenceSubject)
	}
	if criterion.ReworkCount != 0 || criterion.HeldReason != "" || criterion.Result != models.TaskCompletionResultPending {
		t.Fatalf("forged state kept: %+v", criterion)
	}
	if criterion.OwnerKind != completionGateActorHuman || criterion.OwnerID != "user-1" || criterion.Description == "" {
		t.Fatalf("owner/description = %+v", criterion)
	}
}

func TestCreateTaskWithInvalidCriteriaCreatesNothing(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := t.Context()
	wf := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "gate-create-invalid")
	_, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "gate-create-invalid", WorkflowID: wf, Title: "Gated",
		CompletionCriteria: []models.TaskCompletionCriterion{{
			ID: "ci", Kind: models.TaskCompletionCriterionCIAtHead,
			Config: models.TaskCompletionCriterionConfig{ProtectedPaths: []string{"[unclosed"}},
		}},
	})
	if err == nil {
		t.Fatal("create accepted an invalid protected path")
	}
	tasks, listErr := repo.ListTasks(ctx, wf)
	if listErr != nil || len(tasks) != 0 {
		t.Fatalf("tasks after refused create = %d, %v", len(tasks), listErr)
	}
}

// @covers AC-TASKS-COMPLETION-006.8
func TestSetCriteriaRefusesTheTasksOwnRuns(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := t.Context()
	wf := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "gate-self")
	ensureTaskPRTable(t, repo)
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "gate-self", WorkflowID: wf, Title: "Gated"})
	if err != nil {
		t.Fatal(err)
	}
	request := SetTaskCompletionCriteriaRequest{Criteria: []models.TaskCompletionCriterion{{
		ID: "ci", Kind: models.TaskCompletionCriterionCIAtHead,
	}}}
	ownRun := models.WithCompletionGateCaller(ctx, models.CompletionGateCaller{AgentProfileID: "reviewer", RunID: "run-1", TaskID: created.Task.ID})
	if _, err := svc.SetTaskCompletionCriteria(ownRun, created.Task.ID, request); !errors.Is(err, repoerrors.ErrTaskCompletionSelfVerification) {
		t.Fatalf("own run set err = %v", err)
	}
	other := models.WithCompletionGateCaller(ctx, models.CompletionGateCaller{AgentProfileID: "ceo", RunID: "run-2", TaskID: "parent"})
	if _, err := svc.SetTaskCompletionCriteria(other, created.Task.ID, request); err != nil {
		t.Fatalf("other run set err = %v", err)
	}
}

func TestRecordTaskCompletionEvaluationPassesThrough(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wf := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "gate-eval")
	ensureTaskPRTable(t, repo)
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "gate-eval", WorkflowID: wf, Title: "Gated",
		CompletionCriteria: []models.TaskCompletionCriterion{{ID: "ci", Kind: models.TaskCompletionCriterionCIAtHead}}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.DB().ExecContext(ctx, `INSERT INTO github_task_prs (id, task_id, head_sha) VALUES ('pr-1', ?, 'h1')`, created.Task.ID); err != nil {
		t.Fatal(err)
	}
	outcome, err := svc.RecordTaskCompletionEvaluation(ctx, models.TaskCompletionEvaluationChange{
		TaskID: created.Task.ID, CriterionID: "ci", CriterionRevision: 1, SubjectRevision: "pr-1@h1",
		Result: models.TaskCompletionResultPass, VerifierKind: "kandev_ci", VerifierID: "kandev_ci",
	})
	if err != nil || !outcome.Changed || outcome.Snapshot.Blocked {
		t.Fatalf("evaluation err=%v outcome=%+v", err, outcome)
	}
}

// ensureTaskPRTable creates the GitHub-owned PR link table that host criteria
// read; production creates it through the GitHub store.
func ensureTaskPRTable(t *testing.T, repo interface {
	DB() *sql.DB
}) {
	t.Helper()
	if _, err := repo.DB().Exec(`CREATE TABLE IF NOT EXISTS github_task_prs (id TEXT PRIMARY KEY, task_id TEXT NOT NULL,
		head_sha TEXT NOT NULL, state TEXT NOT NULL DEFAULT 'open', detached_at TIMESTAMP)`); err != nil {
		t.Fatalf("create github_task_prs: %v", err)
	}
}
