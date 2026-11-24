package repository

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jmoiron/sqlx"

	"github.com/kandev/kandev/internal/db/dialect"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
)

// OfficeDefinitionWrite replaces (or, with Create, creates) an Office copy
// and its complete step set. Steps carry their final IDs; existing step IDs
// are updated in place and the rest are inserted.
type OfficeDefinitionWrite struct {
	WorkspaceID       string
	WorkflowID        string
	Create            bool
	CreatedByUserID   string
	ExpectedUpdatedAt *time.Time
	Name              string
	Description       string
	Prompt            string
	AgentProfileID    string
	Steps             []*models.WorkflowStep
}

// OfficeDefinitionResult describes a committed definition save.
type OfficeDefinitionResult struct {
	WorkspaceID string
	UpdatedAt   time.Time
	Created     []*models.WorkflowStep
	Updated     []*models.WorkflowStep
	Deleted     []*models.WorkflowStep
}

// OfficeDefinitionCheck validates the locked target before any write.
// selected reports whether the workflow is the workspace's selected Office
// workflow; existing is the stored step set.
type OfficeDefinitionCheck func(row *OfficeWorkflowRow, selected bool, existing []*models.WorkflowStep) []models.OfficeWorkflowProblem

// SaveOfficeWorkflowDefinition applies an Office definition in one
// transaction under the Office workflow lock order (workspace, then
// workflow). check and the removed-step task guard run on the locked state;
// a reported problem refuses the whole write and is returned with a nil
// error.
func (r *Repository) SaveOfficeWorkflowDefinition(
	ctx context.Context, w OfficeDefinitionWrite, check OfficeDefinitionCheck,
) (*OfficeDefinitionResult, []models.OfficeWorkflowProblem, error) {
	tx, err := r.db.BeginTxx(ctx, r.officeCompletionTxOptions())
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	selected, found, err := r.lockOfficeWorkspace(ctx, tx, w.WorkspaceID)
	if err != nil {
		return nil, nil, err
	}
	if !found || selected == "" {
		return nil, []models.OfficeWorkflowProblem{{Code: models.OfficeProblemRequiresOfficeWorkspace}}, nil
	}
	now := time.Now().UTC()
	row, existing, problems, err := r.prepareOfficeDefinitionTarget(ctx, tx, w, now)
	if err != nil || len(problems) > 0 {
		return nil, problems, err
	}
	if problems := check(row, selected == w.WorkflowID, existing); len(problems) > 0 {
		return nil, problems, nil
	}
	result, problems, err := r.writeOfficeDefinitionSteps(ctx, tx, w, existing, now)
	if err != nil || len(problems) > 0 {
		return nil, problems, err
	}
	if !w.Create {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			UPDATE workflows SET name = ?, description = ?, prompt = ?, agent_profile_id = ?, updated_at = ?
			WHERE id = ?
		`), w.Name, w.Description, w.Prompt, w.AgentProfileID, now, w.WorkflowID); err != nil {
			return nil, nil, fmt.Errorf("update office workflow: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	result.WorkspaceID, result.UpdatedAt = w.WorkspaceID, now
	return result, nil, nil
}

// prepareOfficeDefinitionTarget inserts the workflow row for a create, or
// locks and loads the existing workflow for a replace.
func (r *Repository) prepareOfficeDefinitionTarget(
	ctx context.Context, tx *sqlx.Tx, w OfficeDefinitionWrite, now time.Time,
) (*OfficeWorkflowRow, []*models.WorkflowStep, []models.OfficeWorkflowProblem, error) {
	if w.Create {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO workflows (
				created_by_user_id, id, workspace_id, name, description, prompt, agent_profile_id, workflow_template_id,
				sort_order, hidden, style, source, source_path, is_office, is_system, created_at, updated_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, NULL,
				(SELECT COALESCE(MAX(sort_order), -1) + 1 FROM workflows WHERE workspace_id = ?),
				0, ?, ?, '', 1, 0, ?, ?)
		`), w.CreatedByUserID, w.WorkflowID, w.WorkspaceID, w.Name, w.Description, w.Prompt, w.AgentProfileID,
			w.WorkspaceID, taskmodels.WorkflowStyleOffice, taskmodels.WorkflowSourceManual, now, now); err != nil {
			return nil, nil, nil, fmt.Errorf("insert office workflow: %w", err)
		}
		return &OfficeWorkflowRow{ID: w.WorkflowID, WorkspaceID: w.WorkspaceID, IsOffice: true,
			Source: taskmodels.WorkflowSourceManual}, nil, nil, nil
	}
	row, err := r.lockOfficeWorkflowRow(ctx, tx, w.WorkflowID)
	if err != nil {
		return nil, nil, nil, err
	}
	if row == nil || row.WorkspaceID != w.WorkspaceID {
		return nil, nil, nil, fmt.Errorf("%w: %s", repoerrors.ErrWorkflowNotFound, w.WorkflowID)
	}
	if w.ExpectedUpdatedAt != nil {
		var updatedAt time.Time
		if err := tx.QueryRowxContext(ctx, tx.Rebind(`SELECT updated_at FROM workflows WHERE id = ?`), w.WorkflowID).Scan(&updatedAt); err != nil {
			return nil, nil, nil, err
		}
		if !updatedAt.Equal(*w.ExpectedUpdatedAt) {
			return nil, nil, nil, repoerrors.ErrTaskVersionConflict
		}
	}
	existing, err := r.listStepsTx(ctx, tx, w.WorkflowID)
	return row, existing, nil, err
}

// writeOfficeDefinitionSteps refuses removing a step that holds or queues
// tasks, then deletes removed steps, updates kept steps and inserts new ones.
// Start flags are cleared first so the single-start index never sees two.
func (r *Repository) writeOfficeDefinitionSteps(
	ctx context.Context, tx *sqlx.Tx, w OfficeDefinitionWrite, existing []*models.WorkflowStep, now time.Time,
) (*OfficeDefinitionResult, []models.OfficeWorkflowProblem, error) {
	kept := make(map[string]bool, len(w.Steps))
	for _, step := range w.Steps {
		kept[step.ID] = true
	}
	result := &OfficeDefinitionResult{}
	var removed []*models.WorkflowStep
	for _, step := range existing {
		if !kept[step.ID] {
			removed = append(removed, step)
		}
	}
	if problems, err := r.removedStepsHoldingTasks(ctx, tx, removed); err != nil || len(problems) > 0 {
		return nil, problems, err
	}
	if _, err := tx.ExecContext(ctx, tx.Rebind(
		`UPDATE workflow_steps SET is_start_step = 0 WHERE workflow_id = ?`), w.WorkflowID); err != nil {
		return nil, nil, err
	}
	for _, step := range removed {
		if _, err := tx.ExecContext(ctx, tx.Rebind(`DELETE FROM workflow_steps WHERE id = ?`), step.ID); err != nil {
			return nil, nil, fmt.Errorf("delete step %s: %w", step.ID, err)
		}
		result.Deleted = append(result.Deleted, step)
	}
	existingIDs := make(map[string]bool, len(existing))
	for _, step := range existing {
		existingIDs[step.ID] = true
	}
	for _, step := range w.Steps {
		step.WorkflowID, step.UpdatedAt = w.WorkflowID, now
		if existingIDs[step.ID] {
			if err := r.writeOfficeStep(ctx, tx, step, false); err != nil {
				return nil, nil, err
			}
			result.Updated = append(result.Updated, step)
			continue
		}
		step.CreatedAt = now
		if err := r.writeOfficeStep(ctx, tx, step, true); err != nil {
			return nil, nil, err
		}
		result.Created = append(result.Created, step)
	}
	return result, nil, nil
}

func (r *Repository) removedStepsHoldingTasks(
	ctx context.Context, tx *sqlx.Tx, removed []*models.WorkflowStep,
) ([]models.OfficeWorkflowProblem, error) {
	var problems []models.OfficeWorkflowProblem
	for _, step := range removed {
		var held bool
		if err := tx.QueryRowxContext(ctx, tx.Rebind(`
			SELECT EXISTS (SELECT 1 FROM tasks WHERE workflow_step_id = ? OR queued_for_step_id = ?)
		`), step.ID, step.ID).Scan(&held); err != nil {
			return nil, fmt.Errorf("check tasks on step %s: %w", step.ID, err)
		}
		if held {
			problems = append(problems, models.OfficeWorkflowProblem{Code: models.OfficeProblemStepHoldsTasks, StepName: step.Name})
		}
	}
	return problems, nil
}

func (r *Repository) writeOfficeStep(ctx context.Context, tx *sqlx.Tx, step *models.WorkflowStep, insert bool) error {
	eventsJSON, err := json.Marshal(step.Events)
	if err != nil {
		return fmt.Errorf("marshal events: %w", err)
	}
	target, err := marshalSessionTarget(step.SessionTarget)
	if err != nil {
		return err
	}
	values := []interface{}{step.Name, step.Position, step.Color, step.Prompt, string(eventsJSON),
		dialect.BoolToInt(step.AllowManualMove), dialect.BoolToInt(step.IsStartStep), dialect.BoolToInt(step.ShowInCommandPanel),
		step.AutoArchiveAfterHours, step.AgentProfileID,
		taskmodels.NormalizeWorkflowProfileSessionStartPolicy(string(step.ProfileSessionStartPolicy)),
		taskmodels.NormalizeWorkflowProfileSessionEndPolicy(string(step.ProfileSessionEndPolicy)),
		dialect.BoolToInt(step.DisableUnclassifiedFallback), normalizeStageType(step.StageType),
		dialect.BoolToInt(step.AutoAdvanceRequiresSignal), dialect.BoolToInt(step.CancelTriggersTurnComplete),
		dialect.BoolToInt(step.CompleteTaskOnEnter), step.WIPLimit, step.PullFromStepID, target, step.UpdatedAt}
	if insert {
		_, err = tx.ExecContext(ctx, tx.Rebind(`
			INSERT INTO workflow_steps (
				name, position, color, prompt, events, allow_manual_move, is_start_step, show_in_command_panel,
				auto_archive_after_hours, agent_profile_id, profile_session_start_policy, profile_session_end_policy,
				disable_unclassified_fallback, stage_type, auto_advance_requires_signal, cancel_triggers_turn_complete,
				complete_task_on_enter, wip_limit, pull_from_step_id, session_target, updated_at,
				id, workflow_id, created_at
			) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		`), append(values, step.ID, step.WorkflowID, step.CreatedAt)...)
	} else {
		_, err = tx.ExecContext(ctx, tx.Rebind(`
			UPDATE workflow_steps SET
				name = ?, position = ?, color = ?, prompt = ?, events = ?, allow_manual_move = ?, is_start_step = ?,
				show_in_command_panel = ?, auto_archive_after_hours = ?, agent_profile_id = ?,
				profile_session_start_policy = ?, profile_session_end_policy = ?, disable_unclassified_fallback = ?,
				stage_type = ?, auto_advance_requires_signal = ?, cancel_triggers_turn_complete = ?,
				complete_task_on_enter = ?, wip_limit = ?, pull_from_step_id = ?, session_target = ?, updated_at = ?
			WHERE id = ? AND workflow_id = ?
		`), append(values, step.ID, step.WorkflowID)...)
	}
	if err != nil {
		return fmt.Errorf("write step %s: %w", step.ID, err)
	}
	return nil
}
