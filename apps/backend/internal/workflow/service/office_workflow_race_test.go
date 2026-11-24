package service

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"

	"github.com/google/uuid"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
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

// raceCopy creates an Office copy and a broken replacement of its steps.
func (f *officeFixture) raceCopy(t *testing.T, workspaceID string) (*OfficeWorkflowSaveResult, []*models.WorkflowStep) {
	t.Helper()
	created := f.createCopy(t, workspaceID)
	broken := make([]*models.WorkflowStep, 0, len(created.Steps))
	for _, step := range created.Steps {
		copied := *step
		broken = append(broken, &copied)
	}
	broken[1].CompleteTaskOnEnter = false
	return created, broken
}

// assertOneWins requires exactly one of a selection and a broken save to
// succeed, the other to be refused with the expected Office problem, and the
// selected workflow to pass validation.
func (f *officeFixture) assertOneWins(t *testing.T, workspaceID, copyID string, selectErr, saveErr error) {
	t.Helper()
	ctx := context.Background()
	switch {
	case selectErr == nil:
		require.Error(t, saveErr, "both the selection and the broken save succeeded")
		assert.Equal(t, []string{models.OfficeProblemFinalStepNotCompleting}, refusalCodes(t, saveErr))
		assert.Equal(t, copyID, f.selected(t, workspaceID))
	case saveErr == nil:
		assert.Equal(t, []string{models.OfficeProblemFinalStepNotCompleting}, refusalCodes(t, selectErr))
		assert.NotEqual(t, copyID, f.selected(t, workspaceID))
	default:
		t.Fatalf("both operations failed: select=%v save=%v", selectErr, saveErr)
	}
	if f.selected(t, workspaceID) == copyID {
		steps, err := f.svc.repo.ListStepsByWorkflow(ctx, copyID)
		require.NoError(t, err)
		require.Empty(t, models.ValidateOfficeWorkflow(steps, f.svc.officeProfileExists))
	}
}

// @covers AC-OFFICE-WORKFLOW-SELECT-006.5
func TestPostgresSelectionAndSaveSerialize(t *testing.T) {
	runSelectionSaveRace(t, newPostgresOfficeFixture(t))
}

// @covers AC-OFFICE-WORKFLOW-SELECT-006.5
func TestSQLiteSelectionAndSaveSerialize(t *testing.T) {
	runSelectionSaveRace(t, newOfficeFixture(t))
}

func runSelectionSaveRace(t *testing.T, f *officeFixture) {
	ctx := context.Background()
	selectCopy := func(workspaceID, copyID string) error {
		return f.svc.SelectOfficeWorkflow(ctx, workspaceID, copyID)
	}
	saveBroken := func(copyID string, broken []*models.WorkflowStep) error {
		_, err := f.svc.SaveOfficeWorkflowDefinition(ctx, OfficeWorkflowDefinition{
			WorkflowID: copyID, Name: "Office copy", Steps: broken,
		})
		return err
	}

	t.Run("select then save", func(t *testing.T) {
		f.officeWorkspace(t, "ws-select-first")
		created, broken := f.raceCopy(t, "ws-select-first")
		selectErr := selectCopy("ws-select-first", created.WorkflowID)
		f.assertOneWins(t, "ws-select-first", created.WorkflowID, selectErr, saveBroken(created.WorkflowID, broken))
	})
	t.Run("save then select", func(t *testing.T) {
		f.officeWorkspace(t, "ws-save-first")
		created, broken := f.raceCopy(t, "ws-save-first")
		saveErr := saveBroken(created.WorkflowID, broken)
		f.assertOneWins(t, "ws-save-first", created.WorkflowID, selectCopy("ws-save-first", created.WorkflowID), saveErr)
	})
	t.Run("concurrent", func(t *testing.T) {
		for round := 0; round < 10; round++ {
			workspaceID := fmt.Sprintf("ws-race-%d", round)
			f.officeWorkspace(t, workspaceID)
			created, broken := f.raceCopy(t, workspaceID)
			var selectErr, saveErr error
			var wg sync.WaitGroup
			wg.Add(2)
			go func() {
				defer wg.Done()
				selectErr = selectCopy(workspaceID, created.WorkflowID)
			}()
			go func() {
				defer wg.Done()
				saveErr = saveBroken(created.WorkflowID, broken)
			}()
			wg.Wait()
			f.assertOneWins(t, workspaceID, created.WorkflowID, selectErr, saveErr)
		}
	})
}
