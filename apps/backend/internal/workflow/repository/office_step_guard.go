package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/workflow/models"
)

type officeStepGuardKey struct{}

// WithOfficeStepGuard marks a single-step write so its transaction refuses
// the built-in Office workflow and any workspace's selected Office workflow.
// The check runs after the write's locks, so it sees a selection that
// committed after the caller's earlier check.
func WithOfficeStepGuard(ctx context.Context) context.Context {
	return context.WithValue(ctx, officeStepGuardKey{}, true)
}

func officeStepGuarded(ctx context.Context) bool {
	guarded, _ := ctx.Value(officeStepGuardKey{}).(bool)
	return guarded
}

// OfficeStepWriteRefusedError reports a guarded single-step write that
// targeted a workflow Office protects. Code is an Office problem code.
type OfficeStepWriteRefusedError struct {
	Code string
}

func (e *OfficeStepWriteRefusedError) Error() string {
	return "office workflow refused: " + e.Code
}

type rowQuerier interface {
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// recheckOfficeStepGuard refuses a guarded write to workflowID inside tx.
func (r *Repository) recheckOfficeStepGuard(ctx context.Context, tx rowQuerier, workflowID string) error {
	if !officeStepGuarded(ctx) || workflowID == "" {
		return nil
	}
	officeSchema, err := r.hasOfficeSchema(ctx, tx)
	if err != nil || !officeSchema {
		return err
	}
	var isOffice, isSystem, selected bool
	err = tx.QueryRowContext(ctx, r.db.Rebind(`
		SELECT COALESCE(w.is_office, 0) <> 0, COALESCE(w.is_system, 0) <> 0,
			EXISTS (SELECT 1 FROM workspaces ws WHERE ws.office_workflow_id = w.id)
		FROM workflows w WHERE w.id = ?`), workflowID).Scan(&isOffice, &isSystem, &selected)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("recheck office step guard: %w", err)
	}
	switch {
	case isOffice && isSystem:
		return &OfficeStepWriteRefusedError{Code: models.OfficeProblemBuiltInReadOnly}
	case selected:
		return &OfficeStepWriteRefusedError{Code: models.OfficeProblemSelectedRequiresDefinitionSave}
	}
	return nil
}

// hasOfficeSchema reports whether the database carries workspaces, and so
// Office selection. Standalone workflow schemas have no Office workflows.
func (r *Repository) hasOfficeSchema(ctx context.Context, tx rowQuerier) (bool, error) {
	query := `SELECT EXISTS (SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'workspaces')`
	if dialect.IsPostgres(r.db.DriverName()) {
		query = `SELECT EXISTS (SELECT 1 FROM information_schema.tables WHERE table_schema = current_schema() AND table_name = 'workspaces')`
	}
	var exists bool
	if err := tx.QueryRowContext(ctx, query).Scan(&exists); err != nil {
		return false, fmt.Errorf("recheck office step guard: %w", err)
	}
	return exists, nil
}

// recheckOfficeStepGuardForStep resolves the step's workflow inside tx and
// applies recheckOfficeStepGuard.
func (r *Repository) recheckOfficeStepGuardForStep(ctx context.Context, tx rowQuerier, stepID string) error {
	if !officeStepGuarded(ctx) {
		return nil
	}
	var workflowID string
	err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT workflow_id FROM workflow_steps WHERE id = ?`), stepID).Scan(&workflowID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("recheck office step guard: %w", err)
	}
	return r.recheckOfficeStepGuard(ctx, tx, workflowID)
}
