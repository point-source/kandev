package service

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

// officeFixture runs the workflow service against the production task and
// workflow schemas on one database, as the backend does.
type officeFixture struct {
	svc   *Service
	tasks *tasksqlite.Repository
	db    *sqlx.DB
}

func newOfficeFixture(t *testing.T) *officeFixture {
	t.Helper()
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "office-workflow.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	taskRepo, err := tasksqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	wfRepo, err := repository.NewWithDB(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	svc := NewService(wfRepo, log)
	t.Cleanup(func() { _ = svc.Close() })
	svc.SetAgentProfileFuncs(func(id string) *models.AgentProfilePortable {
		if id == "profile-ok" {
			return &models.AgentProfilePortable{AgentName: "claude"}
		}
		return nil
	}, nil)
	return &officeFixture{svc: svc, tasks: taskRepo, db: sqlxDB}
}

// officeWorkspace creates a workspace with the built-in Office workflow.
func (f *officeFixture) officeWorkspace(t *testing.T, id string) string {
	t.Helper()
	ctx := context.Background()
	require.NoError(t, f.tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: id, Name: id, OwnerID: "u-1"}))
	builtin, err := f.tasks.EnsureOfficeWorkflow(ctx, id)
	require.NoError(t, err)
	return builtin
}

func (f *officeFixture) workflow(t *testing.T, workspaceID, id string, mutate func(*taskmodels.Workflow)) {
	t.Helper()
	wf := &taskmodels.Workflow{ID: id, WorkspaceID: workspaceID, Name: id}
	if mutate != nil {
		mutate(wf)
	}
	require.NoError(t, f.tasks.CreateWorkflow(context.Background(), wf))
}

// validSteps creates a minimal workflow Office can drive: a signal-gated work
// start step that moves to a completing final step.
func (f *officeFixture) validSteps(t *testing.T, workflowID string) {
	t.Helper()
	ctx := context.Background()
	done := workflowID + "-done"
	require.NoError(t, f.svc.repo.CreateStep(ctx, &models.WorkflowStep{
		ID: workflowID + "-work", WorkflowID: workflowID, Name: "Work", Position: 0, IsStartStep: true,
		StageType: models.StageTypeWork, AutoAdvanceRequiresSignal: true,
		Events: models.StepEvents{OnTurnComplete: []models.OnTurnCompleteAction{
			{Type: models.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": done}},
		}},
	}))
	require.NoError(t, f.svc.repo.CreateStep(ctx, &models.WorkflowStep{
		ID: done, WorkflowID: workflowID, Name: "Done", Position: 1, StageType: models.StageTypeCustom,
		CompleteTaskOnEnter: true,
	}))
}

func (f *officeFixture) selected(t *testing.T, workspaceID string) string {
	t.Helper()
	ws, err := f.tasks.GetWorkspace(context.Background(), workspaceID)
	require.NoError(t, err)
	return ws.OfficeWorkflowID
}

func (f *officeFixture) isOffice(t *testing.T, workflowID string) bool {
	t.Helper()
	wf, err := f.tasks.GetWorkflow(context.Background(), workflowID)
	require.NoError(t, err)
	return wf.IsOffice
}

func refusalCodes(t *testing.T, err error) []string {
	t.Helper()
	refusal, ok := AsOfficeWorkflowRefusal(err)
	require.True(t, ok, "error %v is not an Office workflow refusal", err)
	codes := make([]string, 0, len(refusal.Problems))
	for _, p := range refusal.Problems {
		codes = append(codes, p.Code)
	}
	return codes
}
