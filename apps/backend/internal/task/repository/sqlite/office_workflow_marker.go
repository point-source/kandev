package sqlite

import (
	"context"
	"fmt"
	"time"
)

// backfillOfficeWorkflowMarker marks the workflows that are Office workflows
// under the pre-marker rule: every workflow a workspace references through
// office_workflow_id and every system Office Default row. It never clears the
// marker, so re-running it is a no-op.
func (r *Repository) backfillOfficeWorkflowMarker() error {
	if _, err := r.db.ExecContext(r.migrationContext(), r.db.Rebind(`
		UPDATE workflows SET is_office = 1, updated_at = ?
		WHERE COALESCE(is_office, 0) = 0 AND (
			id IN (
				SELECT office_workflow_id FROM workspaces
				WHERE COALESCE(office_workflow_id, '') != ''
			)
			OR (COALESCE(is_system, 0) = 1 AND workflow_template_id = ?)
		)
	`), time.Now().UTC(), templateIDOfficeDefault); err != nil {
		return fmt.Errorf("backfill office workflow marker: %w", err)
	}
	return nil
}

// ListOfficeWorkflowIDs returns the IDs of every Office workflow across all
// workspaces: marked workflows plus each workspace's selected workflow.
// Kanban listings exclude these workflows.
func (r *Repository) ListOfficeWorkflowIDs(ctx context.Context) (map[string]struct{}, error) {
	rows, err := r.ro.QueryContext(ctx, `
		SELECT id FROM workflows WHERE is_office = 1
		UNION
		SELECT office_workflow_id FROM workspaces WHERE COALESCE(office_workflow_id, '') != ''`)
	if err != nil {
		return nil, fmt.Errorf("list office workflows: %w", err)
	}
	defer func() { _ = rows.Close() }()
	ids := make(map[string]struct{})
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids[id] = struct{}{}
	}
	return ids, rows.Err()
}

// IsOfficeWorkflowInWorkspace reports whether workflowID is an Office
// workflow owned by workspaceID.
func (r *Repository) IsOfficeWorkflowInWorkspace(ctx context.Context, workspaceID, workflowID string) (bool, error) {
	var exists bool
	err := r.ro.QueryRowContext(ctx, r.ro.Rebind(`
		SELECT EXISTS (
			SELECT 1 FROM workflows WHERE id = ? AND workspace_id = ? AND is_office = 1
		) OR EXISTS (
			SELECT 1 FROM workspaces WHERE id = ? AND office_workflow_id = ?
		)
	`), workflowID, workspaceID, workspaceID, workflowID).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check office workflow: %w", err)
	}
	return exists, nil
}
