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
		SELECT default_criteria_json, default_verification_limit, default_rework_limit, verification_limit_ceiling, rework_limit_ceiling,
			default_review_wait_limit
		FROM office_completion_gate_settings WHERE workspace_id = ?
	`), workspaceID).Scan(&criteriaJSON, &settings.DefaultVerificationLimit, &settings.DefaultReworkLimit,
		&settings.VerificationLimitCeiling, &settings.ReworkLimitCeiling, &settings.DefaultReviewWaitLimit)
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
			workspace_id, default_criteria_json, default_verification_limit, default_rework_limit,
			verification_limit_ceiling, rework_limit_ceiling, default_review_wait_limit, updated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(workspace_id) DO UPDATE SET
			default_criteria_json = excluded.default_criteria_json,
			default_verification_limit = excluded.default_verification_limit,
			default_rework_limit = excluded.default_rework_limit,
			verification_limit_ceiling = excluded.verification_limit_ceiling,
			rework_limit_ceiling = excluded.rework_limit_ceiling,
			default_review_wait_limit = excluded.default_review_wait_limit,
			updated_at = excluded.updated_at
	`), settings.WorkspaceID, string(encoded), settings.DefaultVerificationLimit, settings.DefaultReworkLimit,
		settings.VerificationLimitCeiling, settings.ReworkLimitCeiling, settings.DefaultReviewWaitLimit, time.Now().UTC())
	return err
}
