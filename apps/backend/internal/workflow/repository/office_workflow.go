package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
)

// OfficeWorkflowRow is the selection-relevant view of a workflow row.
type OfficeWorkflowRow struct {
	ID          string `db:"id"`
	WorkspaceID string `db:"workspace_id"`
	Name        string `db:"name"`
	Source      string `db:"source"`
	IsOffice    bool   `db:"is_office"`
	IsSystem    bool   `db:"is_system"`
	Hidden      bool   `db:"hidden"`
	TaskCount   int    `db:"task_count"`
}

// OfficeWorkflowCheck decides whether a locked workflow and its steps may
// become the selected Office workflow. A non-empty result refuses the write.
type OfficeWorkflowCheck func(row *OfficeWorkflowRow, steps []*models.WorkflowStep) []models.OfficeWorkflowProblem

const officeWorkflowRowColumns = `
	w.id, w.workspace_id, w.name, COALESCE(w.source, 'manual') AS source,
	COALESCE(w.is_office, 0) AS is_office, COALESCE(w.is_system, 0) AS is_system,
	COALESCE(w.hidden, 0) AS hidden,
	(SELECT COUNT(*) FROM tasks t WHERE t.workflow_id = w.id) AS task_count`

// ListOfficeWorkflowRows returns the workspace's selected Office workflow ID
// and every workflow row in the workspace.
func (r *Repository) ListOfficeWorkflowRows(ctx context.Context, workspaceID string) (string, []*OfficeWorkflowRow, error) {
	var selected string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COALESCE(office_workflow_id, '') FROM workspaces WHERE id = ?`), workspaceID).Scan(&selected)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil, fmt.Errorf("%w: %s", repoerrors.ErrWorkspaceNotFound, workspaceID)
	}
	if err != nil {
		return "", nil, err
	}
	var rows []*OfficeWorkflowRow
	if err := sqlx.SelectContext(ctx, r.ro, &rows, r.ro.Rebind(`
		SELECT `+officeWorkflowRowColumns+`
		FROM workflows w WHERE w.workspace_id = ?
		ORDER BY w.sort_order, w.created_at, w.id
	`), workspaceID); err != nil {
		return "", nil, err
	}
	return selected, rows, nil
}

// SelectOfficeWorkflow makes workflowID the workspace's selected Office
// workflow and marks it Office, in one transaction that locks the workspace
// row and then the workflow row. check runs against the locked state; any
// problem it or the workspace lookup reports refuses the write and is
// returned with a nil error.
func (r *Repository) SelectOfficeWorkflow(
	ctx context.Context, workspaceID, workflowID string, check OfficeWorkflowCheck,
) ([]models.OfficeWorkflowProblem, error) {
	tx, err := r.db.BeginTxx(ctx, r.officeCompletionTxOptions())
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()

	selected, found, err := r.lockOfficeWorkspace(ctx, tx, workspaceID)
	if err != nil {
		return nil, err
	}
	if !found || selected == "" {
		return []models.OfficeWorkflowProblem{{Code: models.OfficeProblemRequiresOfficeWorkspace}}, nil
	}
	row, err := r.lockOfficeWorkflowRow(ctx, tx, workflowID)
	if err != nil {
		return nil, err
	}
	if row == nil || row.WorkspaceID != workspaceID {
		return []models.OfficeWorkflowProblem{{Code: models.OfficeProblemNotInWorkspace}}, nil
	}
	steps, err := r.listStepsTx(ctx, tx, workflowID)
	if err != nil {
		return nil, err
	}
	if problems := check(row, steps); len(problems) > 0 {
		return problems, nil
	}
	now := time.Now().UTC()
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE workspaces SET office_workflow_id = ?, updated_at = ? WHERE id = ?`), workflowID, now, workspaceID); err != nil {
		return nil, fmt.Errorf("set office workflow: %w", err)
	}
	if !row.IsOffice {
		if _, err := tx.ExecContext(ctx, tx.Rebind(
			`UPDATE workflows SET is_office = 1, style = 'office', updated_at = ? WHERE id = ?`), now, workflowID); err != nil {
			return nil, fmt.Errorf("mark office workflow: %w", err)
		}
	}
	return nil, tx.Commit()
}

// postgresRowLock locks the selected rows until the transaction ends.
const postgresRowLock = ` FOR UPDATE`

// lockOfficeWorkspace locks the workspace row for an Office workflow write.
// Every Office workflow write locks the workspace before the workflow so
// selection and definition saves serialize without deadlocking.
func (r *Repository) lockOfficeWorkspace(ctx context.Context, tx *sqlx.Tx, workspaceID string) (string, bool, error) {
	query := `SELECT COALESCE(office_workflow_id, '') FROM workspaces WHERE id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += postgresRowLock
	}
	var selected string
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), workspaceID).Scan(&selected)
	if errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, fmt.Errorf("lock workspace: %w", err)
	}
	return selected, true, nil
}

func (r *Repository) lockOfficeWorkflowRow(ctx context.Context, tx *sqlx.Tx, workflowID string) (*OfficeWorkflowRow, error) {
	query := `SELECT ` + officeWorkflowRowColumns + ` FROM workflows w WHERE w.id = ?`
	if dialect.IsPostgres(r.db.DriverName()) {
		query += ` FOR UPDATE OF w`
	}
	row := &OfficeWorkflowRow{}
	err := tx.QueryRowxContext(ctx, tx.Rebind(query), workflowID).StructScan(row)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("lock workflow: %w", err)
	}
	return row, nil
}

func (r *Repository) listStepsTx(ctx context.Context, tx *sqlx.Tx, workflowID string) ([]*models.WorkflowStep, error) {
	rows, err := tx.QueryContext(ctx, tx.Rebind(`
		SELECT `+stepSelectColumns+`
		FROM workflow_steps WHERE workflow_id = ? ORDER BY position
	`), workflowID)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	return r.scanSteps(rows)
}

// WorkflowWorkspaceID returns the workspace that owns workflowID, or
// sql.ErrNoRows when the workflow does not exist.
func (r *Repository) WorkflowWorkspaceID(ctx context.Context, workflowID string) (string, error) {
	var workspaceID string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`SELECT workspace_id FROM workflows WHERE id = ?`), workflowID).Scan(&workspaceID)
	return workspaceID, err
}

// WorkspaceOfficeWorkflowID returns the workspace's selected Office workflow,
// or "" when the workspace has no Office.
func (r *Repository) WorkspaceOfficeWorkflowID(ctx context.Context, workspaceID string) (string, error) {
	var selected string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(
		`SELECT COALESCE(office_workflow_id, '') FROM workspaces WHERE id = ?`), workspaceID).Scan(&selected)
	if errors.Is(err, sql.ErrNoRows) {
		return "", fmt.Errorf("%w: %s", repoerrors.ErrWorkspaceNotFound, workspaceID)
	}
	return selected, err
}
