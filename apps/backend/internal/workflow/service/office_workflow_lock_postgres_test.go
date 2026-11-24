package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	internaldb "github.com/kandev/kandev/internal/db"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/testutil"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

func newPostgresOfficeFixture(t *testing.T) *officeFixture {
	t.Helper()
	dsn := testutil.PostgresDSNFromEnv(t)
	schema := "kandev_test_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	setup, err := sqlx.Open("pgx", dsn)
	require.NoError(t, err)
	_, err = setup.Exec("CREATE SCHEMA " + schema)
	require.NoError(t, err)
	require.NoError(t, setup.Close())
	t.Cleanup(func() {
		if cleanup, cerr := sqlx.Open("pgx", dsn); cerr == nil {
			_, _ = cleanup.Exec("DROP SCHEMA IF EXISTS " + schema + " CASCADE")
			_ = cleanup.Close()
		}
	})
	raw, err := internaldb.OpenPostgres(testutil.PostgresDSNWithSearchPath(t, dsn, schema), 4, 4)
	require.NoError(t, err)
	db := sqlx.NewDb(raw, "pgx")
	t.Cleanup(func() { _ = db.Close() })
	taskRepo, err := tasksqlite.NewWithDB(db, db, nil)
	require.NoError(t, err)
	wfRepo, err := repository.NewWithDB(db, db, nil)
	require.NoError(t, err)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	svc := NewService(wfRepo, log)
	t.Cleanup(func() { _ = svc.Close() })
	return &officeFixture{svc: svc, tasks: taskRepo, db: db}
}

// @covers AC-OFFICE-WORKFLOW-SELECT-006.5
func TestPostgresSelectionAndSaveSerialize(t *testing.T) {
	f := newPostgresOfficeFixture(t)
	ctx := context.Background()
	for round := 0; round < 10; round++ {
		workspaceID := fmt.Sprintf("ws-race-%d", round)
		builtin := f.officeWorkspace(t, workspaceID)
		created := f.createCopy(t, workspaceID)
		broken := make([]*models.WorkflowStep, 0, len(created.Steps))
		for _, step := range created.Steps {
			copied := *step
			broken = append(broken, &copied)
		}
		broken[1].CompleteTaskOnEnter = false

		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			_ = f.svc.SelectOfficeWorkflow(ctx, workspaceID, created.WorkflowID)
		}()
		go func() {
			defer wg.Done()
			_, _ = f.svc.SaveOfficeWorkflowDefinition(ctx, OfficeWorkflowDefinition{
				WorkflowID: created.WorkflowID, Name: "Office copy", Steps: broken,
			})
		}()
		wg.Wait()

		selected := f.selected(t, workspaceID)
		if selected == builtin {
			continue
		}
		require.Equal(t, created.WorkflowID, selected)
		steps, err := f.svc.repo.ListStepsByWorkflow(ctx, created.WorkflowID)
		require.NoError(t, err)
		require.Empty(t, models.ValidateOfficeWorkflow(steps, f.svc.officeProfileExists),
			"round %d: the selected Office workflow fails validation", round)
	}
}
