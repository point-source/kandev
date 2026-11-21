package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/kandev/kandev/internal/office/models"
)

// GetCompletionGateSettings returns the workspace's stored completion-gate
// settings, or ok=false when none were written.
func (r *Repository) GetCompletionGateSettings(ctx context.Context, workspaceID string) (models.CompletionGateSettings, bool, error) {
	var settings models.CompletionGateSettings
	var criteriaJSON string
	err := r.ro.QueryRowxContext(ctx, r.ro.Rebind(`
		SELECT default_criteria_json, default_max_attempts, default_max_rework, ceiling_attempts, ceiling_rework
		FROM office_completion_gate_settings WHERE workspace_id = ?
	`), workspaceID).Scan(&criteriaJSON, &settings.DefaultMaxAttempts, &settings.DefaultMaxRework,
		&settings.CeilingAttempts, &settings.CeilingRework)
	if errors.Is(err, sql.ErrNoRows) {
		return models.CompletionGateSettings{WorkspaceID: workspaceID}, false, nil
	}
	if err != nil {
		return settings, false, err
	}
	settings.WorkspaceID = workspaceID
	if err := json.Unmarshal([]byte(criteriaJSON), &settings.DefaultCriteria); err != nil {
		return settings, false, err
	}
	return settings, true, nil
}

// SetCompletionGateSettings replaces the workspace's completion-gate settings.
func (r *Repository) SetCompletionGateSettings(ctx context.Context, settings models.CompletionGateSettings) error {
	criteria := settings.DefaultCriteria
	if criteria == nil {
		criteria = []models.CompletionGateCriterionSpec{}
	}
	encoded, err := json.Marshal(criteria)
	if err != nil {
		return err
	}
	_, err = r.db.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO office_completion_gate_settings (
			workspace_id, default_criteria_json, default_max_attempts, default_max_rework,
			ceiling_attempts, ceiling_rework, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			default_criteria_json = excluded.default_criteria_json,
			default_max_attempts = excluded.default_max_attempts,
			default_max_rework = excluded.default_max_rework,
			ceiling_attempts = excluded.ceiling_attempts,
			ceiling_rework = excluded.ceiling_rework,
			updated_at = excluded.updated_at
	`), settings.WorkspaceID, string(encoded), settings.DefaultMaxAttempts, settings.DefaultMaxRework,
		settings.CeilingAttempts, settings.CeilingRework, time.Now().UTC())
	return err
}
