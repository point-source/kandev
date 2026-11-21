package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/kandev/kandev/internal/task/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const completionGateSummaryBatchSize = 100

type completionGateSummaryTask struct {
	updatedAt           time.Time
	revision            int64
	stateRevision       int64
	criteria            []models.TaskCompletionCriterion
	requiredAssignments int
	assignedChildren    int
}

type completionGateEvidenceKey struct {
	taskID string
	id     string
}

func (r *Repository) GetTaskCompletionGateSummaries(
	ctx context.Context,
	taskIDs []string,
) (models.TaskCompletionGateSummaryBatch, error) {
	result := models.TaskCompletionGateSummaryBatch{
		ByTaskID:       make(map[string]*models.TaskCompletionGateSummaryObservation, len(taskIDs)),
		MissingTaskIDs: []string{},
	}
	ids := uniqueCompletionGateTaskIDs(taskIDs)
	for _, chunk := range chunkIDs(ids, completionGateSummaryBatchSize) {
		observations, missing, err := r.readTaskCompletionGateSummaryChunk(ctx, chunk)
		if err != nil {
			return models.TaskCompletionGateSummaryBatch{}, err
		}
		for taskID, observation := range observations {
			result.ByTaskID[taskID] = observation
		}
		result.MissingTaskIDs = append(result.MissingTaskIDs, missing...)
	}
	return result, nil
}

func uniqueCompletionGateTaskIDs(taskIDs []string) []string {
	seen := make(map[string]struct{}, len(taskIDs))
	unique := make([]string, 0, len(taskIDs))
	for _, taskID := range taskIDs {
		if _, exists := seen[taskID]; exists {
			continue
		}
		seen[taskID] = struct{}{}
		unique = append(unique, taskID)
	}
	return unique
}

func (r *Repository) readTaskCompletionGateSummaryChunk(
	ctx context.Context,
	taskIDs []string,
) (map[string]*models.TaskCompletionGateSummaryObservation, []string, error) {
	tx, err := r.beginTaskCompletionGateRead(ctx)
	if err != nil {
		return nil, nil, err
	}
	defer func() { _ = tx.Rollback() }()

	idsPlaceholder, idArgs := buildInPlaceholders(taskIDs)
	tasks := make(map[string]*completionGateSummaryTask, len(taskIDs))
	identityQuery := `SELECT parent.id, parent.updated_at, parent.state, COALESCE(parent.metadata, '{}'), ` + assignedRoutineChildrenSQL("parent") + ` FROM tasks parent WHERE parent.id IN (` + idsPlaceholder + `)`
	if err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(identityQuery), idArgs, func(rows *sql.Rows) error {
		var taskID string
		var updatedAt time.Time
		var state v1.TaskState
		var raw string
		var assigned int
		if err := rows.Scan(&taskID, &updatedAt, &state, &raw, &assigned); err != nil {
			return err
		}
		required := 0
		if state != v1.TaskStateCompleted {
			var err error
			required, err = requiredSubtaskCount(raw)
			if err != nil {
				return err
			}
		}
		tasks[taskID] = &completionGateSummaryTask{updatedAt: updatedAt, requiredAssignments: required, assignedChildren: assigned}
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("load completion-gate task identities: %w", err)
	}

	missing := missingCompletionGateTaskIDs(taskIDs, tasks)
	if len(tasks) == 0 {
		if err := tx.Commit(); err != nil {
			return nil, nil, err
		}
		return map[string]*models.TaskCompletionGateSummaryObservation{}, missing, nil
	}

	placeholders, args := buildInPlaceholders(taskIDs)
	setQuery := `SELECT task_id, revision, state_revision FROM task_completion_sets WHERE task_id IN (` + placeholders + `)`
	if err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(setQuery), args, func(rows *sql.Rows) error {
		var taskID string
		var revision, stateRevision int64
		if err := rows.Scan(&taskID, &revision, &stateRevision); err != nil {
			return err
		}
		if task := tasks[taskID]; task != nil {
			task.revision, task.stateRevision = revision, stateRevision
		}
		return nil
	}); err != nil {
		return nil, nil, fmt.Errorf("load completion-gate revisions: %w", err)
	}

	criteriaQuery := `
		SELECT task_id, criterion_revision, verified_revision, evidence_kind, evidence_id, evidence_revision,
			kind, held_reason, result
		FROM task_completion_criteria
		WHERE task_id IN (` + placeholders + `)
		ORDER BY task_id, criterion_id`
	if err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(criteriaQuery), args, func(rows *sql.Rows) error {
		return scanCompletionGateBatchCriterion(rows, tasks)
	}); err != nil {
		return nil, nil, fmt.Errorf("load completion-gate criteria: %w", err)
	}

	prHeads, err := r.readCompletionGateBatchPRHeads(ctx, tx, tasks, placeholders, args)
	if err != nil {
		return nil, nil, fmt.Errorf("load completion-gate pull-request evidence: %w", err)
	}
	executions, err := r.readCompletionGateBatchExecutions(ctx, tx, tasks, placeholders, args)
	if err != nil {
		return nil, nil, fmt.Errorf("load completion-gate execution evidence: %w", err)
	}
	headSets, err := r.readCompletionGateBatchPRHeadSets(ctx, tx, tasks, placeholders, args)
	if err != nil {
		return nil, nil, fmt.Errorf("load completion-gate pull-request head sets: %w", err)
	}
	for key, revision := range headSets {
		prHeads[key] = revision
	}

	observations := completionGateBatchObservations(tasks, prHeads, executions)
	if err := tx.Commit(); err != nil {
		return nil, nil, err
	}
	return observations, missing, nil
}

func readCompletionGateSummaryRows(
	ctx context.Context,
	tx *sql.Tx,
	query string,
	args []interface{},
	visit func(*sql.Rows) error,
) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return err
	}
	for rows.Next() {
		if err := visit(rows); err != nil {
			_ = rows.Close()
			return err
		}
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	return rows.Close()
}

func (r *Repository) readCompletionGateBatchPRHeads(
	ctx context.Context,
	tx *sql.Tx,
	tasks map[string]*completionGateSummaryTask,
	placeholders string,
	args []interface{},
) (map[completionGateEvidenceKey]string, error) {
	if !completionGateBatchHasEvidence(tasks, models.TaskCompletionEvidenceGitHubPRHead) {
		return map[completionGateEvidenceKey]string{}, nil
	}
	query := `
		SELECT c.task_id, c.evidence_id, pr.head_sha
		FROM task_completion_criteria c
		JOIN github_task_prs pr ON pr.id = c.evidence_id AND pr.task_id = c.task_id
		WHERE c.task_id IN (` + placeholders + `)
		  AND c.evidence_kind = ?
		  AND c.verified_revision = c.criterion_revision`
	queryArgs := append(append([]interface{}(nil), args...), models.TaskCompletionEvidenceGitHubPRHead)
	observed := make(map[completionGateEvidenceKey]string)
	err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(query), queryArgs, func(rows *sql.Rows) error {
		var key completionGateEvidenceKey
		var head string
		if err := rows.Scan(&key.taskID, &key.id, &head); err != nil {
			return err
		}
		observed[key] = head
		return nil
	})
	return observed, err
}

func (r *Repository) readCompletionGateBatchExecutions(
	ctx context.Context,
	tx *sql.Tx,
	tasks map[string]*completionGateSummaryTask,
	placeholders string,
	args []interface{},
) (map[completionGateEvidenceKey]struct{}, error) {
	if !completionGateBatchHasEvidence(tasks, models.TaskCompletionEvidenceExecution) {
		return map[completionGateEvidenceKey]struct{}{}, nil
	}
	query := `
		SELECT c.task_id, c.evidence_id
		FROM task_completion_criteria c
		JOIN task_session_turns turn
		  ON turn.id = c.evidence_id AND turn.task_id = c.task_id AND turn.completed_at IS NOT NULL
		WHERE c.task_id IN (` + placeholders + `)
		  AND c.evidence_kind = ?
		  AND c.verified_revision = c.criterion_revision`
	queryArgs := append(append([]interface{}(nil), args...), models.TaskCompletionEvidenceExecution)
	observed := make(map[completionGateEvidenceKey]struct{})
	err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(query), queryArgs, func(rows *sql.Rows) error {
		var key completionGateEvidenceKey
		if err := rows.Scan(&key.taskID, &key.id); err != nil {
			return err
		}
		observed[key] = struct{}{}
		return nil
	})
	return observed, err
}

func completionGateBatchHasEvidence(tasks map[string]*completionGateSummaryTask, kind string) bool {
	for _, task := range tasks {
		for _, criterion := range task.criteria {
			if criterion.Evidence != nil && criterion.Evidence.Subject.Kind == kind &&
				!completionCriterionBlocked(criterion, true) {
				return true
			}
		}
	}
	return false
}

func completionGateBatchEvidenceCurrent(
	criterion models.TaskCompletionCriterion,
	taskUpdatedAt time.Time,
	prHeads map[completionGateEvidenceKey]string,
	executions map[completionGateEvidenceKey]struct{},
	taskID string,
) bool {
	if completionCriterionBlocked(criterion, true) {
		return false
	}
	subject := criterion.Evidence.Subject
	key := completionGateEvidenceKey{taskID: taskID, id: subject.ID}
	switch subject.Kind {
	case models.TaskCompletionEvidenceTaskRevision:
		return completionEvidenceSubjectCurrent(subject, &taskUpdatedAt, "", true)
	case models.TaskCompletionEvidenceGitHubPRHead, models.TaskCompletionEvidenceGitHubPRHeads:
		head, exists := prHeads[completionGateEvidenceKey{taskID: taskID, id: subject.Kind + ":" + subject.ID}]
		if subject.Kind == models.TaskCompletionEvidenceGitHubPRHead {
			head, exists = prHeads[key]
		}
		return completionEvidenceSubjectCurrent(subject, nil, head, exists)
	case models.TaskCompletionEvidenceExecution:
		_, exists := executions[key]
		return completionEvidenceSubjectCurrent(subject, nil, "", exists)
	case models.TaskCompletionEvidenceArtifact:
		return completionEvidenceSubjectCurrent(subject, nil, "", true)
	default:
		return false
	}
}

func completionGateBatchObservations(tasks map[string]*completionGateSummaryTask, prHeads map[completionGateEvidenceKey]string, executions map[completionGateEvidenceKey]struct{}) map[string]*models.TaskCompletionGateSummaryObservation {
	observations := make(map[string]*models.TaskCompletionGateSummaryObservation, len(tasks))
	for taskID, task := range tasks {
		if task.revision == 0 && len(task.criteria) == 0 && task.requiredAssignments == 0 {
			observations[taskID] = nil
			continue
		}
		observation := &models.TaskCompletionGateSummaryObservation{
			Revision:      task.revision,
			StateRevision: task.stateRevision,
			CriteriaCount: len(task.criteria),
		}
		observation.WeakestConfidence = models.WeakestTaskCompletionConfidence(task.criteria)
		for _, criterion := range task.criteria {
			if criterion.HeldReason != "" {
				observation.HeldCount++
			}
			switch criterion.Result {
			case models.TaskCompletionResultFail:
				observation.FailedCount++
			case models.TaskCompletionResultCannotEvaluate:
				observation.CannotEvaluateCount++
			}
			evidenceCurrent := completionGateBatchEvidenceCurrent(criterion, task.updatedAt, prHeads, executions, taskID)
			if completionCriterionBlocked(criterion, evidenceCurrent) {
				observation.BlockerCount++
			} else {
				observation.VerifiedCount++
			}
		}
		if task.requiredAssignments > task.assignedChildren {
			observation.BlockerCount++
		}
		observation.Blocked = observation.BlockerCount > 0
		observations[taskID] = observation
	}
	return observations
}

func missingCompletionGateTaskIDs(taskIDs []string, tasks map[string]*completionGateSummaryTask) []string {
	missing := make([]string, 0)
	for _, taskID := range taskIDs {
		if tasks[taskID] == nil {
			missing = append(missing, taskID)
		}
	}
	return missing
}

func scanCompletionGateBatchCriterion(rows *sql.Rows, tasks map[string]*completionGateSummaryTask) error {
	var (
		taskID           string
		criterion        models.TaskCompletionCriterion
		evidenceKind     string
		evidenceID       string
		evidenceRevision string
	)
	if err := rows.Scan(&taskID, &criterion.CriterionRevision, &criterion.VerifiedRevision,
		&evidenceKind, &evidenceID, &evidenceRevision, &criterion.Kind, &criterion.HeldReason, &criterion.Result); err != nil {
		return err
	}
	criterion.Kind = models.NormalizeTaskCompletionCriterionKind(criterion.Kind)
	if evidenceKind != "" {
		criterion.Evidence = &models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{
				Kind: evidenceKind, ID: evidenceID, Revision: evidenceRevision,
			},
		}
	}
	if task := tasks[taskID]; task != nil {
		task.criteria = append(task.criteria, criterion)
	}
	return nil
}

// readCompletionGateBatchPRHeadSets returns the live PR heads revision of each
// task that has verified host-criterion evidence, keyed by
// "github_pr_heads:<task id>".
func (r *Repository) readCompletionGateBatchPRHeadSets(
	ctx context.Context,
	tx *sql.Tx,
	tasks map[string]*completionGateSummaryTask,
	placeholders string,
	args []interface{},
) (map[completionGateEvidenceKey]string, error) {
	if !completionGateBatchHasEvidence(tasks, models.TaskCompletionEvidenceGitHubPRHeads) {
		return map[completionGateEvidenceKey]string{}, nil
	}
	query := `SELECT task_id, id, head_sha FROM github_task_prs WHERE task_id IN (` + placeholders + `) AND detached_at IS NULL`
	byTask := make(map[string][]models.TaskPRHead)
	err := readCompletionGateSummaryRows(ctx, tx, r.ro.Rebind(query), args, func(rows *sql.Rows) error {
		var taskID string
		var head models.TaskPRHead
		if err := rows.Scan(&taskID, &head.ID, &head.HeadSHA); err != nil {
			return err
		}
		byTask[taskID] = append(byTask[taskID], head)
		return nil
	})
	observed := make(map[completionGateEvidenceKey]string, len(byTask))
	for taskID, heads := range byTask {
		observed[completionGateEvidenceKey{taskID: taskID, id: models.TaskCompletionEvidenceGitHubPRHeads + ":" + taskID}] = models.TaskPRHeadsRevision(heads)
	}
	return observed, err
}
