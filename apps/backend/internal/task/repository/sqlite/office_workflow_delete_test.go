package sqlite

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

// The delete transaction re-reads the selection, so a workflow selected after
// the caller's own check is not deleted out from under the workspace.
func TestDeleteWorkflowRefusesSelectedOfficeWorkflow(t *testing.T) {
	repo := newRepoForBuiltinWorkflowTests(t)
	ctx := context.Background()
	createMarkerTestWorkspace(t, repo, "ws-delete")
	if _, err := repo.EnsureOfficeWorkflow(ctx, "ws-delete"); err != nil {
		t.Fatalf("EnsureOfficeWorkflow: %v", err)
	}
	copyID := createMarkerTestWorkflow(t, repo, "ws-delete", "Office copy", true)
	if _, err := repo.db.ExecContext(ctx, repo.db.Rebind(
		`UPDATE workspaces SET office_workflow_id = ? WHERE id = ?`), copyID, "ws-delete"); err != nil {
		t.Fatalf("select copy: %v", err)
	}

	if err := repo.DeleteWorkflow(ctx, copyID); !errors.Is(err, repoerrors.ErrOfficeWorkflowSelected) {
		t.Fatalf("DeleteWorkflow(selected) = %v, want ErrOfficeWorkflowSelected", err)
	}
	if wf, err := repo.GetWorkflow(ctx, copyID); err != nil || wf == nil {
		t.Fatalf("selected workflow should remain, got %v, %v", wf, err)
	}

	unselected := createMarkerTestWorkflow(t, repo, "ws-delete", "Spare copy", true)
	if err := repo.DeleteWorkflow(ctx, unselected); err != nil {
		t.Fatalf("DeleteWorkflow(unselected): %v", err)
	}
}
