package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func createMarkerTestWorkspace(t *testing.T, repo *Repository, id string) {
	t.Helper()
	if err := repo.CreateWorkspace(context.Background(), &models.Workspace{ID: id, Name: id, OwnerID: "u-1"}); err != nil {
		t.Fatalf("CreateWorkspace: %v", err)
	}
}

func createMarkerTestWorkflow(t *testing.T, repo *Repository, workspaceID, name string, office bool) string {
	t.Helper()
	wf := &models.Workflow{WorkspaceID: workspaceID, Name: name, IsOffice: office}
	if err := repo.CreateWorkflow(context.Background(), wf); err != nil {
		t.Fatalf("CreateWorkflow %q: %v", name, err)
	}
	return wf.ID
}

// @covers AC-OFFICE-WORKFLOW-SELECT-001.1
func TestIsFromOfficeAfterSelectionChange(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	createMarkerTestWorkspace(t, repo, "ws-switch")
	builtin, err := repo.EnsureOfficeWorkflow(ctx, "ws-switch")
	if err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}
	copyID := createMarkerTestWorkflow(t, repo, "ws-switch", "Office copy", true)
	kanbanID := createMarkerTestWorkflow(t, repo, "ws-switch", "Kanban", false)

	for _, task := range []*models.Task{
		{ID: "t-old", WorkspaceID: "ws-switch", WorkflowID: builtin, Title: "old", State: "TODO"},
		{ID: "t-kanban", WorkspaceID: "ws-switch", WorkflowID: kanbanID, Title: "kanban", State: "TODO"},
	} {
		if err := repo.CreateTask(ctx, task); err != nil {
			t.Fatalf("CreateTask %s: %v", task.ID, err)
		}
	}
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE workspaces SET office_workflow_id = ? WHERE id = ?`), copyID, "ws-switch"); err != nil {
		t.Fatalf("switch office workflow: %v", err)
	}

	old, err := repo.GetTask(ctx, "t-old")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if !old.IsFromOffice {
		t.Fatal("task on the previously selected Office workflow lost its Office identity")
	}
	kanban, err := repo.GetTask(ctx, "t-kanban")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if kanban.IsFromOffice {
		t.Fatal("Kanban task became an Office task")
	}

	ids, err := repo.ListOfficeWorkflowIDs(ctx)
	if err != nil {
		t.Fatalf("ListOfficeWorkflowIDs: %v", err)
	}
	for _, want := range []string{builtin, copyID} {
		if _, ok := ids[want]; !ok {
			t.Errorf("ListOfficeWorkflowIDs missing %s", want)
		}
	}
	if _, ok := ids[kanbanID]; ok {
		t.Error("ListOfficeWorkflowIDs includes the Kanban workflow")
	}
}

// @covers AC-OFFICE-WORKFLOW-SELECT-001.1
func TestIsFromOfficeIgnoresOfficeWorkflowOfAnotherWorkspace(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	createMarkerTestWorkspace(t, repo, "ws-a")
	createMarkerTestWorkspace(t, repo, "ws-b")
	foreign := createMarkerTestWorkflow(t, repo, "ws-a", "Office copy", true)
	if err := repo.CreateTask(ctx, &models.Task{ID: "t-foreign", WorkspaceID: "ws-b", WorkflowID: foreign, Title: "x", State: "TODO"}); err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	got, err := repo.GetTask(ctx, "t-foreign")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if got.IsFromOffice {
		t.Fatal("task in another workspace classified as Office through a foreign Office workflow")
	}
}

// @covers AC-OFFICE-WORKFLOW-SELECT-001.3
func TestBackfillMarksCurrentOfficeWorkflowsOnly(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	createMarkerTestWorkspace(t, repo, "ws-office")
	createMarkerTestWorkspace(t, repo, "ws-kanban")
	builtin, err := repo.EnsureOfficeWorkflow(ctx, "ws-office")
	if err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}
	stamped := createMarkerTestWorkflow(t, repo, "ws-kanban", "Manually stamped", false)
	plain := createMarkerTestWorkflow(t, repo, "ws-kanban", "Plain", false)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE workspaces SET office_workflow_id = ? WHERE id = ?`), stamped, "ws-kanban"); err != nil {
		t.Fatalf("stamp: %v", err)
	}
	if _, err := repo.db.ExecContext(ctx, `UPDATE workflows SET is_office = 0`); err != nil {
		t.Fatalf("reset marker: %v", err)
	}

	if err := repo.backfillOfficeWorkflowMarker(); err != nil {
		t.Fatalf("backfillOfficeWorkflowMarker: %v", err)
	}

	for id, want := range map[string]bool{builtin: true, stamped: true, plain: false} {
		wf, err := repo.GetWorkflow(ctx, id)
		if err != nil {
			t.Fatalf("GetWorkflow %s: %v", id, err)
		}
		if wf.IsOffice != want {
			t.Errorf("workflow %q IsOffice = %v, want %v", wf.Name, wf.IsOffice, want)
		}
	}
}

func TestEnsureOfficeWorkflowCreatesMarkedWorkflow(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	createMarkerTestWorkspace(t, repo, "ws-new")
	id, err := repo.EnsureOfficeWorkflow(ctx, "ws-new")
	if err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}
	wf, err := repo.GetWorkflow(ctx, id)
	if err != nil {
		t.Fatalf("GetWorkflow: %v", err)
	}
	if !wf.IsOffice || !wf.IsSystem {
		t.Fatalf("built-in Office workflow IsOffice=%v IsSystem=%v, want both true", wf.IsOffice, wf.IsSystem)
	}
	routine, err := repo.EnsureRoutineWorkflow(ctx, "ws-new")
	if err != nil {
		t.Fatalf("EnsureRoutineWorkflow: %v", err)
	}
	rwf, err := repo.GetWorkflow(ctx, routine)
	if err != nil {
		t.Fatalf("GetWorkflow routine: %v", err)
	}
	if rwf.IsOffice {
		t.Fatal("routine workflow must not be marked Office")
	}
}
