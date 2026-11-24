package sqlite

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/google/uuid"
	"github.com/kandev/kandev/internal/db/dialect"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

const (
	maxCompletionResultDetail     = 20
	maxCompletionResultDetailLen  = 200
	maxCompletionProtectedPaths   = 32
	maxCompletionLimit            = 1000
	maxCompletionReviewWaitLimit  = 30 * 24 * 60
	completionGateActionEvaluated = "criterion_evaluated"
	prStateMerged                 = "merged"
)

// completionCriterionColumns is the column list read by every criterion scan.
const completionCriterionColumns = `criterion_id, description, criterion_revision, subject_kind, subject_id,
	verified_revision, evidence_kind, evidence_id, evidence_revision, evidence_summary,
	evidence_reference, verifier_kind, verifier_id, verified_at,
	kind, config_json, owner_kind, owner_id, result, result_reason, result_detail_json,
	result_subject_revision, result_first_seen_at, result_at, attempt_count, rework_count,
	hold_kind, held_at, protected_allowed_revision, protected_allowed_by, rework_keys_json, escalated_at`

func scanCompletionCriterion(row rowScanner) (models.TaskCompletionCriterion, error) {
	var item models.TaskCompletionCriterion
	var evidenceKind, evidenceID, evidenceRevision, evidenceSummary, evidenceReference string
	var configJSON, detailJSON, reworkKeysJSON string
	var verifiedAt, firstSeenAt, resultAt, heldAt, escalatedAt sql.NullTime
	if err := row.Scan(&item.ID, &item.Description, &item.CriterionRevision,
		&item.EvidenceSubject.Kind, &item.EvidenceSubject.ID, &item.VerifiedRevision,
		&evidenceKind, &evidenceID, &evidenceRevision, &evidenceSummary, &evidenceReference,
		&item.VerifierKind, &item.VerifierID, &verifiedAt,
		&item.Kind, &configJSON, &item.OwnerKind, &item.OwnerID, &item.Result, &item.ResultReason, &detailJSON,
		&item.ResultSubjectRevision, &firstSeenAt, &resultAt, &item.AttemptCount, &item.ReworkCount,
		&item.HoldKind, &heldAt, &item.ProtectedAllowedRevision, &item.ProtectedAllowedBy, &reworkKeysJSON, &escalatedAt); err != nil {
		return item, err
	}
	if evidenceKind != "" {
		item.Evidence = &models.TaskCompletionEvidence{
			Subject: models.TaskCompletionEvidenceSubject{Kind: evidenceKind, ID: evidenceID, Revision: evidenceRevision},
			Summary: evidenceSummary, Reference: evidenceReference,
		}
	}
	item.Kind = models.NormalizeTaskCompletionCriterionKind(item.Kind)
	_ = json.Unmarshal([]byte(configJSON), &item.Config)
	_ = json.Unmarshal([]byte(detailJSON), &item.ResultDetail)
	item.VerifiedAt = nullTimePtr(verifiedAt)
	item.ResultFirstSeenAt = nullTimePtr(firstSeenAt)
	item.ResultAt = nullTimePtr(resultAt)
	item.HeldAt = nullTimePtr(heldAt)
	item.EscalatedAt = nullTimePtr(escalatedAt)
	_ = json.Unmarshal([]byte(reworkKeysJSON), &item.ReworkKeys)
	item.VerdictSource = models.TaskCompletionVerdictSource(item)
	return item, nil
}

func nullTimePtr(value sql.NullTime) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}

// taskPRHeads is the live state of a task's linked, non-detached PRs.
type taskPRHeads struct {
	revision  string
	count     int
	anyMerged bool
}

func (r *Repository) readTaskPRHeadsTx(ctx context.Context, tx *sql.Tx, taskID string, lock bool) (taskPRHeads, error) {
	query := `SELECT id, head_sha, state FROM github_task_prs WHERE task_id = ? AND detached_at IS NULL`
	if lock && dialect.IsPostgres(r.db.DriverName()) {
		query += forUpdateClause
	}
	rows, err := tx.QueryContext(ctx, r.db.Rebind(query), taskID)
	if err != nil {
		return taskPRHeads{}, err
	}
	defer func() { _ = rows.Close() }()
	var heads []models.TaskPRHead
	result := taskPRHeads{}
	for rows.Next() {
		var head models.TaskPRHead
		var state string
		if err := rows.Scan(&head.ID, &head.HeadSHA, &state); err != nil {
			return taskPRHeads{}, err
		}
		heads = append(heads, head)
		result.anyMerged = result.anyMerged || state == prStateMerged
	}
	if err := rows.Err(); err != nil {
		return taskPRHeads{}, err
	}
	result.count = len(heads)
	result.revision = models.TaskPRHeadsRevision(heads)
	return result, nil
}

// hostCriterionBlocker classifies why an unmet host-verified criterion blocks.
func hostCriterionBlocker(criterion models.TaskCompletionCriterion, heads taskPRHeads) string {
	switch {
	case criterion.HoldKind != "":
		return models.TaskCompletionBlockerHeld
	case heads.count == 0:
		return models.TaskCompletionBlockerPRMissing
	case heads.anyMerged:
		return models.TaskCompletionBlockerPRMergedUnverified
	case criterion.VerifiedRevision != criterion.CriterionRevision || criterion.Evidence == nil:
		return models.TaskCompletionBlockerUnverified
	default:
		return models.TaskCompletionBlockerEvidenceStale
	}
}

func insertCompletionCriterionTx(ctx context.Context, tx *sql.Tx, rebind func(string) string, taskID string, criterion models.TaskCompletionCriterion) error {
	config, err := json.Marshal(criterion.Config)
	if err != nil {
		return err
	}
	detail, err := json.Marshal(boundedCompletionDetail(criterion.ResultDetail))
	if err != nil {
		return err
	}
	result := criterion.Result
	if result == "" {
		result = models.TaskCompletionResultPending
	}
	reworkKeys, err := json.Marshal(append([]string{}, criterion.ReworkKeys...))
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, rebind(`
		INSERT INTO task_completion_criteria (
			task_id, criterion_id, description, criterion_revision, subject_kind, subject_id,
			verified_revision, evidence_kind, evidence_id, evidence_revision, evidence_summary,
			evidence_reference, verifier_kind, verifier_id, verified_at,
			kind, config_json, owner_kind, owner_id, result, result_reason, result_detail_json,
			result_subject_revision, result_first_seen_at, result_at, attempt_count, rework_count,
			hold_kind, held_at, protected_allowed_revision, protected_allowed_by, rework_keys_json, escalated_at
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`), taskID, criterion.ID, criterion.Description, criterion.CriterionRevision,
		criterion.EvidenceSubject.Kind, criterion.EvidenceSubject.ID, criterion.VerifiedRevision,
		evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Subject.Kind }),
		evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Subject.ID }),
		evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Subject.Revision }),
		evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Summary }),
		evidenceField(criterion.Evidence, func(e models.TaskCompletionEvidence) string { return e.Reference }),
		criterion.VerifierKind, criterion.VerifierID, criterion.VerifiedAt,
		models.NormalizeTaskCompletionCriterionKind(criterion.Kind), string(config), criterion.OwnerKind, criterion.OwnerID,
		result, criterion.ResultReason, string(detail), criterion.ResultSubjectRevision,
		criterion.ResultFirstSeenAt, criterion.ResultAt, criterion.AttemptCount, criterion.ReworkCount,
		criterion.HoldKind, criterion.HeldAt, criterion.ProtectedAllowedRevision, criterion.ProtectedAllowedBy,
		string(reworkKeys), criterion.EscalatedAt)
	return err
}

// insertInitialCompletionCriteriaTx stores criteria supplied with a new task in
// the task's create transaction.
func (r *Repository) insertInitialCompletionCriteriaTx(ctx context.Context, tx *sql.Tx, task *models.Task) error {
	if len(task.CompletionCriteria) == 0 {
		return nil
	}
	criteria := normalizeCompletionCriteria(task.CompletionCriteria)
	if err := validateCompletionCriteriaForTask(task.ID, criteria); err != nil {
		return err
	}
	now := r.nowUTC()
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`
		INSERT INTO task_completion_sets (task_id, workspace_id, revision, updated_at) VALUES (?, ?, 1, ?)
	`), task.ID, task.WorkspaceID, now); err != nil {
		return err
	}
	for _, criterion := range criteria {
		criterion.CriterionRevision = 1
		if err := insertCompletionCriterionTx(ctx, tx, r.db.Rebind, task.ID, criterion); err != nil {
			return err
		}
	}
	details, _ := json.Marshal(criteria)
	return insertCompletionHistoryTx(ctx, tx, r.db.Rebind, models.TaskCompletionGateHistory{
		ID: uuid.NewString(), TaskID: task.ID, WorkspaceID: task.WorkspaceID, Revision: 1,
		Action: "criteria_set", ActorKind: criteria[0].OwnerKind, ActorID: criteria[0].OwnerID,
		Details: string(details), CreatedAt: now,
	})
}

func normalizeCompletionCriteria(criteria []models.TaskCompletionCriterion) []models.TaskCompletionCriterion {
	clean := make([]models.TaskCompletionCriterion, len(criteria))
	for index, criterion := range criteria {
		criterion.ID = strings.TrimSpace(criterion.ID)
		criterion.Description = strings.TrimSpace(criterion.Description)
		criterion.Kind = models.NormalizeTaskCompletionCriterionKind(strings.TrimSpace(criterion.Kind))
		criterion.EvidenceSubject.Kind = strings.TrimSpace(criterion.EvidenceSubject.Kind)
		criterion.EvidenceSubject.ID = strings.TrimSpace(criterion.EvidenceSubject.ID)
		criterion.Config.ProtectedPaths = normalizeProtectedPaths(criterion.Config.ProtectedPaths)
		clean[index] = criterion
	}
	return clean
}

func normalizeProtectedPaths(paths []string) []string {
	if len(paths) == 0 {
		return nil
	}
	clean := make([]string, 0, len(paths))
	for _, path := range paths {
		clean = append(clean, strings.TrimSpace(path))
	}
	slices.Sort(clean)
	return slices.Compact(clean)
}

// validateCompletionCriteriaForTask applies the set-wide rules and the
// per-kind rules that need the owning task's identity. Every refusal wraps
// ErrTaskCompletionCriteriaInvalid.
func validateCompletionCriteriaForTask(taskID string, criteria []models.TaskCompletionCriterion) error {
	if err := checkCompletionCriteriaForTask(taskID, criteria); err != nil {
		return fmt.Errorf("%w: %w", repoerrors.ErrTaskCompletionCriteriaInvalid, err)
	}
	return nil
}

func checkCompletionCriteriaForTask(taskID string, criteria []models.TaskCompletionCriterion) error {
	if err := validateCompletionCriteria(criteria); err != nil {
		return err
	}
	hostKinds := map[string]bool{}
	for _, criterion := range criteria {
		if err := validateCompletionCriterionKind(taskID, criterion); err != nil {
			return err
		}
		kind := models.NormalizeTaskCompletionCriterionKind(criterion.Kind)
		if kind == models.TaskCompletionCriterionAttested {
			continue
		}
		if hostKinds[kind] {
			return fmt.Errorf("a task carries at most one %s criterion", kind)
		}
		hostKinds[kind] = true
	}
	return nil
}

func validateCompletionCriterionKind(taskID string, criterion models.TaskCompletionCriterion) error {
	kind := models.NormalizeTaskCompletionCriterionKind(criterion.Kind)
	if err := validateCompletionLimits(criterion.Config); err != nil {
		return err
	}
	if err := validateCompletionKindConfig(kind, criterion); err != nil || kind == models.TaskCompletionCriterionAttested {
		return err
	}
	if criterion.EvidenceSubject.Kind != models.TaskCompletionEvidenceGitHubPRHeads || criterion.EvidenceSubject.ID != taskID {
		return fmt.Errorf("host-verified completion criterion must name the task's PR heads")
	}
	return validateProtectedPaths(criterion.Config.ProtectedPaths)
}

// validateCompletionKindConfig applies each kind's configuration rules: a
// check names the github runner, a review names none and protects no paths,
// an approval carries only a rework limit, and an attested criterion carries
// no host-verified configuration.
func validateCompletionKindConfig(kind string, criterion models.TaskCompletionCriterion) error {
	config := criterion.Config
	switch kind {
	case models.TaskCompletionCriterionAttested:
		if criterion.EvidenceSubject.Kind == models.TaskCompletionEvidenceGitHubPRHeads || !noHostConfig(config) {
			return fmt.Errorf("attested completion criterion is invalid")
		}
	case models.TaskCompletionCriterionCheck:
		if config.Runner != models.TaskCompletionCheckRunnerGitHub || config.ReviewWaitLimit != 0 {
			return fmt.Errorf("check completion criterion must name the github runner")
		}
	case models.TaskCompletionCriterionReview:
		if config.Runner != "" || len(config.ProtectedPaths) > 0 {
			return fmt.Errorf("review completion criterion has no runner or protected paths")
		}
	case models.TaskCompletionCriterionApproval:
		if !noHostConfig(config) || config.VerificationLimit != 0 {
			return fmt.Errorf("approval completion criterion carries only a rework limit")
		}
	default:
		return fmt.Errorf("completion criterion kind %q is not supported", kind)
	}
	return nil
}

// noHostConfig reports a configuration without a runner, protected paths or a
// review wait limit.
func noHostConfig(config models.TaskCompletionCriterionConfig) bool {
	return config.Runner == "" && len(config.ProtectedPaths) == 0 && config.ReviewWaitLimit == 0
}

func validateCompletionLimits(config models.TaskCompletionCriterionConfig) error {
	if config.VerificationLimit < 0 || config.VerificationLimit > maxCompletionLimit ||
		config.ReworkLimit < 0 || config.ReworkLimit > maxCompletionLimit ||
		config.ReviewWaitLimit < 0 || config.ReviewWaitLimit > maxCompletionReviewWaitLimit {
		return fmt.Errorf("completion criterion limits are out of range")
	}
	return nil
}

func validateProtectedPaths(paths []string) error {
	if len(paths) > maxCompletionProtectedPaths {
		return fmt.Errorf("a criterion can have at most %d protected paths", maxCompletionProtectedPaths)
	}
	for _, pattern := range paths {
		if pattern == "" || len(pattern) > 256 || !doublestar.ValidatePattern(pattern) {
			return fmt.Errorf("protected path %q is invalid", pattern)
		}
	}
	return nil
}

func boundedCompletionDetail(detail []string) []string {
	if len(detail) > maxCompletionResultDetail {
		detail = detail[:maxCompletionResultDetail]
	}
	bounded := make([]string, len(detail))
	for index, item := range detail {
		if len(item) > maxCompletionResultDetailLen {
			item = item[:maxCompletionResultDetailLen]
		}
		bounded[index] = item
	}
	return bounded
}

// RecordTaskCompletionEvaluation stores one in-process verifier result for a
// host-verified criterion while the criterion and the live PR heads still
// match what the verifier observed.
func (r *Repository) RecordTaskCompletionEvaluation(ctx context.Context, change models.TaskCompletionEvaluationChange) (*models.TaskCompletionEvaluationOutcome, error) {
	if !validCompletionResult(change.Result) {
		return nil, fmt.Errorf("completion result %q is invalid", change.Result)
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	locked, err := r.lockHostCriterionTx(ctx, tx, change.TaskID, change.CriterionID, change.CriterionRevision, change.SubjectRevision)
	if err != nil {
		return nil, err
	}
	mergedBeforePass(locked, &change)
	workspaceID, revision := locked.workspaceID, locked.revision
	next, changed := applyCompletionEvaluation(locked.criterion, change, r.nowUTC())
	if changed {
		if err := r.writeCompletionCriterionStateTx(ctx, tx, change.TaskID, next); err != nil {
			return nil, err
		}
		if err := r.bumpCompletionStateTx(ctx, tx, change.TaskID); err != nil {
			return nil, err
		}
		details, _ := json.Marshal(map[string]any{
			"criterion_id": change.CriterionID, "result": change.Result, "reason": change.Reason,
			"subject_revision": change.SubjectRevision, "detail": next.ResultDetail, "hold": change.Hold,
			"verdict_source": models.TaskCompletionVerdictSource(next),
		})
		if err := insertCompletionHistoryTx(ctx, tx, r.db.Rebind, models.TaskCompletionGateHistory{
			ID: uuid.NewString(), TaskID: change.TaskID, WorkspaceID: workspaceID, Revision: revision,
			Action: completionGateActionEvaluated, ActorKind: "system", ActorID: change.VerifierID,
			Reason: change.Reason, Details: string(details), CreatedAt: r.nowUTC(),
		}); err != nil {
			return nil, err
		}
	}
	snapshot, err := r.readTaskCompletionGateTx(ctx, tx, change.TaskID, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &models.TaskCompletionEvaluationOutcome{Snapshot: snapshot, Criterion: findCriterion(snapshot, change.CriterionID), Changed: changed}, nil
}

func validCompletionResult(result string) bool {
	switch result {
	case models.TaskCompletionResultPending, models.TaskCompletionResultPass,
		models.TaskCompletionResultFail, models.TaskCompletionResultCannotEvaluate:
		return true
	default:
		return false
	}
}

func findCriterion(snapshot *models.TaskCompletionGateSnapshot, criterionID string) models.TaskCompletionCriterion {
	for _, criterion := range snapshot.Criteria {
		if criterion.ID == criterionID {
			return criterion
		}
	}
	return models.TaskCompletionCriterion{}
}

// lockHostCriterionTx serializes on the task row and returns the current
// host-verified criterion when its revision and the observed subject are live.
// lockedHostCriterion is a host-verified criterion read under the task lock.
type lockedHostCriterion struct {
	criterion   models.TaskCompletionCriterion
	workspaceID string
	revision    int64
	heads       taskPRHeads
}

func (r *Repository) lockHostCriterionTx(
	ctx context.Context, tx *sql.Tx, taskID, criterionID string, criterionRevision int64, subjectRevision string,
) (lockedHostCriterion, error) {
	locked := lockedHostCriterion{}
	workspaceID, err := r.lockCompletionTask(ctx, tx, taskID)
	if err != nil {
		return locked, err
	}
	locked.workspaceID = workspaceID
	if err := tx.QueryRowContext(ctx, r.db.Rebind(`SELECT revision FROM task_completion_sets WHERE task_id = ?`), taskID).Scan(&locked.revision); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return locked, repoerrors.ErrTaskCompletionEvidenceChanged
		}
		return locked, err
	}
	criterion, err := scanCompletionCriterion(tx.QueryRowContext(ctx, r.db.Rebind(
		`SELECT `+completionCriterionColumns+` FROM task_completion_criteria WHERE task_id = ? AND criterion_id = ?`), taskID, criterionID))
	if errors.Is(err, sql.ErrNoRows) {
		return locked, repoerrors.ErrTaskCompletionEvidenceChanged
	}
	if err != nil {
		return locked, err
	}
	if !models.IsHostVerifiedCriterionKind(criterion.Kind) || criterion.CriterionRevision != criterionRevision {
		return locked, repoerrors.ErrTaskCompletionEvidenceChanged
	}
	locked.criterion = criterion
	if subjectRevision != "" {
		heads, err := r.readTaskPRHeadsTx(ctx, tx, taskID, true)
		if err != nil {
			return locked, err
		}
		if heads.count == 0 || heads.revision != subjectRevision {
			return locked, repoerrors.ErrTaskCompletionEvidenceChanged
		}
		locked.heads = heads
	}
	return locked, nil
}

// bumpCompletionStateTx advances the gate state revision after a result, hold
// or control change so status summaries republish.
func (r *Repository) bumpCompletionStateTx(ctx context.Context, tx *sql.Tx, taskID string) error {
	_, err := tx.ExecContext(ctx, r.db.Rebind(`UPDATE task_completion_sets SET state_revision = state_revision + 1 WHERE task_id = ?`), taskID)
	return err
}

// mergedBeforePass refuses a first pass after a linked PR merged: a merged
// head can no longer be proven before completion.
func mergedBeforePass(locked lockedHostCriterion, change *models.TaskCompletionEvaluationChange) {
	criterion := locked.criterion
	alreadyPassed := criterion.VerifiedRevision == criterion.CriterionRevision && criterion.Evidence != nil &&
		criterion.Evidence.Subject.Revision == change.SubjectRevision
	if change.Result == models.TaskCompletionResultPass && locked.heads.anyMerged && !alreadyPassed {
		change.Result, change.Reason = models.TaskCompletionResultPending, models.TaskCompletionBlockerPRMergedUnverified
	}
}

// applyCompletionEvaluation computes the next criterion state. It reports
// changed=false when the observation repeats the stored one.
func applyCompletionEvaluation(current models.TaskCompletionCriterion, change models.TaskCompletionEvaluationChange, now time.Time) (models.TaskCompletionCriterion, bool) {
	if personVerdictStands(current, change) {
		return current, false
	}
	detail := boundedCompletionDetail(change.Detail)
	releases := change.ReleaseHold != "" && change.ReleaseHold == current.HoldKind && change.Hold == ""
	holdUnchanged := (change.Hold == "" || change.Hold == current.HoldKind) && !releases
	if current.Result == change.Result && current.ResultReason == change.Reason &&
		current.ResultSubjectRevision == change.SubjectRevision && holdUnchanged &&
		slices.Equal(current.ResultDetail, detail) {
		return current, false
	}
	next := current
	if next.ResultSubjectRevision != change.SubjectRevision || next.ResultFirstSeenAt == nil {
		next.ResultFirstSeenAt = &now
	}
	next.Result, next.ResultReason, next.ResultDetail = change.Result, change.Reason, detail
	next.ResultSubjectRevision = change.SubjectRevision
	next.ResultAt = &now
	applyEvaluationVerification(&next, change, now)
	applyEvaluationCounters(&next, change, releases, now)
	return next, true
}

// personVerdictStands reports a verifier observation at the heads a person
// already sent back, accepted or approved: the person's verdict stands until
// the heads move, even when the verifier read its inputs before the person
// acted.
func personVerdictStands(current models.TaskCompletionCriterion, change models.TaskCompletionEvaluationChange) bool {
	if current.ResultSubjectRevision != change.SubjectRevision || change.VerifierKind == models.TaskCompletionVerifierHuman {
		return false
	}
	switch current.ResultReason {
	case models.TaskCompletionReasonSentBack, models.TaskCompletionReasonAccepted, models.TaskCompletionReasonApproved:
		return true
	}
	return false
}

// applyEvaluationVerification keeps the invariant that a criterion is
// verified exactly when its latest result is pass.
func applyEvaluationVerification(next *models.TaskCompletionCriterion, change models.TaskCompletionEvaluationChange, now time.Time) {
	if change.Result != models.TaskCompletionResultPass {
		next.VerifiedRevision, next.Evidence = 0, nil
		next.VerifierKind, next.VerifierID, next.VerifiedAt = "", "", nil
		return
	}
	next.VerifiedRevision = next.CriterionRevision
	next.Evidence = &models.TaskCompletionEvidence{
		Subject: models.TaskCompletionEvidenceSubject{Kind: models.TaskCompletionEvidenceGitHubPRHeads, ID: next.EvidenceSubject.ID, Revision: change.SubjectRevision},
		Summary: strings.TrimSpace(change.EvidenceSummary), Reference: strings.TrimSpace(change.EvidenceReference),
	}
	next.VerifierKind, next.VerifierID, next.VerifiedAt = change.VerifierKind, change.VerifierID, &now
}

func applyEvaluationCounters(next *models.TaskCompletionCriterion, change models.TaskCompletionEvaluationChange, releases bool, now time.Time) {
	if change.CountAttempt {
		next.AttemptCount++
	}
	if change.CountRework {
		next.ReworkCount++
	}
	if change.Hold != "" && change.Hold != next.HoldKind {
		next.HoldKind, next.HeldAt = change.Hold, &now
	}
	if releases {
		next.HoldKind, next.HeldAt = "", nil
	}
}

func (r *Repository) writeCompletionCriterionStateTx(ctx context.Context, tx *sql.Tx, taskID string, criterion models.TaskCompletionCriterion) error {
	if _, err := tx.ExecContext(ctx, r.db.Rebind(`DELETE FROM task_completion_criteria WHERE task_id = ? AND criterion_id = ?`), taskID, criterion.ID); err != nil {
		return err
	}
	return insertCompletionCriterionTx(ctx, tx, r.db.Rebind, taskID, criterion)
}

// ApplyTaskCompletionControl applies one recovery action to a host-verified
// criterion and records it in the gate history.
func (r *Repository) ApplyTaskCompletionControl(ctx context.Context, change models.TaskCompletionControlChange) (*models.TaskCompletionControlOutcome, error) {
	if strings.TrimSpace(change.ActorID) == "" {
		return nil, fmt.Errorf("completion control actor is required")
	}
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback() }()
	locked, err := r.lockControlledCriterionTx(ctx, tx, change)
	if err != nil {
		return nil, err
	}
	criterion := locked.criterion
	next, err := applyCompletionControl(criterion, change, r.nowUTC())
	if err != nil {
		return nil, err
	}
	claimed := (change.Action == models.TaskCompletionControlCountRework &&
		(next.ReworkCount > criterion.ReworkCount || criterion.HoldKind != next.HoldKind)) ||
		(change.Action == models.TaskCompletionControlHold && criterion.HoldKind != next.HoldKind)
	if !controlLeftUnchanged(criterion, next, change.Action) {
		if err := r.writeControlledCriterionTx(ctx, tx, change, next, locked); err != nil {
			return nil, err
		}
	}
	snapshot, err := r.readTaskCompletionGateTx(ctx, tx, change.TaskID, false)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return &models.TaskCompletionControlOutcome{Snapshot: snapshot, Claimed: claimed}, nil
}

// lockControlledCriterionTx locks the criterion a control applies to, bound to
// the heads the person saw for heads-bound controls.
func (r *Repository) lockControlledCriterionTx(ctx context.Context, tx *sql.Tx, change models.TaskCompletionControlChange) (lockedHostCriterion, error) {
	subject := ""
	if isSubjectBoundControl(change.Action) || change.RebindFromSubject != "" {
		if change.SubjectRevision == "" {
			return lockedHostCriterion{}, repoerrors.ErrTaskCompletionEvidenceChanged
		}
		subject = change.SubjectRevision
	}
	return r.lockHostCriterionTx(ctx, tx, change.TaskID, change.CriterionID, change.CriterionRevision, subject)
}

// writeControlledCriterionTx stores a control's result and its history entry.
func (r *Repository) writeControlledCriterionTx(
	ctx context.Context, tx *sql.Tx, change models.TaskCompletionControlChange,
	next models.TaskCompletionCriterion, locked lockedHostCriterion,
) error {
	if err := r.writeCompletionCriterionStateTx(ctx, tx, change.TaskID, next); err != nil {
		return err
	}
	if err := r.bumpCompletionStateTx(ctx, tx, change.TaskID); err != nil {
		return err
	}
	details, _ := json.Marshal(map[string]any{
		"criterion_id": change.CriterionID, "subject_revision": change.SubjectRevision,
		"verification_limit": next.Config.VerificationLimit, "rework_limit": next.Config.ReworkLimit,
		"hold_kind": next.HoldKind, "verdict_source": models.TaskCompletionVerdictSource(next),
	})
	return insertCompletionHistoryTx(ctx, tx, r.db.Rebind, models.TaskCompletionGateHistory{
		ID: uuid.NewString(), TaskID: change.TaskID, WorkspaceID: locked.workspaceID, Revision: locked.revision,
		Action: change.Action, ActorKind: change.ActorKind, ActorID: change.ActorID,
		Reason: strings.TrimSpace(change.Reason), Details: string(details), CreatedAt: r.nowUTC(),
	})
}

func isSubjectBoundControl(action string) bool {
	switch action {
	case models.TaskCompletionControlAllowProtected, models.TaskCompletionControlSendBack,
		models.TaskCompletionControlAccept, models.TaskCompletionControlApprove:
		return true
	}
	return false
}

// controlLeftUnchanged reports a hold or release that found the criterion
// already in the requested state; it writes nothing and records no history.
func controlLeftUnchanged(before, after models.TaskCompletionCriterion, action string) bool {
	if action != models.TaskCompletionControlHold && action != models.TaskCompletionControlReleaseHold {
		return false
	}
	return before.HoldKind == after.HoldKind && slices.Equal(before.ResultDetail, after.ResultDetail) &&
		before.ResultSubjectRevision == after.ResultSubjectRevision
}

const (
	holdProtectedFilesChanged    = "protected_files_changed"
	holdVerificationLimitReached = "verification_limit_reached"
	holdReworkLimitReached       = "rework_limit_reached"
	holdNoReviewerAvailable      = "no_reviewer_available"
	holdReviewWaitLimitReached   = "review_wait_limit_reached"
)

func applyCompletionControl(criterion models.TaskCompletionCriterion, change models.TaskCompletionControlChange, now time.Time) (models.TaskCompletionCriterion, error) {
	next := criterion
	switch change.Action {
	case models.TaskCompletionControlHold, models.TaskCompletionControlReleaseHold:
		return next, applyHoldControl(&next, change, now)
	case models.TaskCompletionControlAllowProtected, models.TaskCompletionControlSendBack,
		models.TaskCompletionControlAccept, models.TaskCompletionControlApprove:
		return next, applyPersonControl(&next, criterion, change, now)
	case models.TaskCompletionControlExtendLimits:
		return next, applyLimitExtension(&next, change)
	case models.TaskCompletionControlCountRework:
		applyReworkCount(&next, change, now)
	case models.TaskCompletionControlMarkEscalated:
		if next.HoldKind != "" && next.EscalatedAt == nil {
			next.EscalatedAt = &now
		}
	default:
		return next, fmt.Errorf("completion control %q is not supported", change.Action)
	}
	return next, nil
}

// applyHoldControl sets a named hold at the given heads, or releases a hold of
// the named kind (any hold when no kind is named).
func applyHoldControl(next *models.TaskCompletionCriterion, change models.TaskCompletionControlChange, now time.Time) error {
	if change.Action == models.TaskCompletionControlReleaseHold {
		if change.HoldKind == "" || change.HoldKind == next.HoldKind {
			next.HoldKind, next.HeldAt, next.EscalatedAt = "", nil, nil
		}
		return nil
	}
	if !knownHoldKind(change.HoldKind) {
		return fmt.Errorf("completion hold kind %q is not supported", change.HoldKind)
	}
	if change.RequireResultReason != "" && next.ResultReason != change.RequireResultReason {
		return nil
	}
	if change.RebindFromSubject != "" && (next.HoldKind != change.HoldKind || next.ResultSubjectRevision != change.RebindFromSubject) {
		return nil
	}
	if next.HoldKind != change.HoldKind {
		next.HoldKind, next.HeldAt, next.EscalatedAt = change.HoldKind, &now, nil
	}
	next.ResultDetail = boundedCompletionDetail(change.Detail)
	if change.SubjectRevision != "" {
		next.ResultSubjectRevision = change.SubjectRevision
	}
	return nil
}

func knownHoldKind(kind string) bool {
	switch kind {
	case holdProtectedFilesChanged, holdVerificationLimitReached, holdReworkLimitReached, holdNoReviewerAvailable, holdReviewWaitLimitReached:
		return true
	}
	return false
}

// ListTaskIDsWithUnmetCompletionCriteria returns open, unarchived tasks that
// have a criterion of kind that has not passed and is not held, oldest result
// first.
func (r *Repository) ListTaskIDsWithUnmetCompletionCriteria(ctx context.Context, query models.TaskCompletionSweepQuery) ([]string, error) {
	if query.Limit <= 0 {
		return nil, nil
	}
	where := []string{"t.archived_at IS NULL", "t.state NOT IN (?, ?, ?)", "c.task_id > ?"}
	args := []interface{}{v1.TaskStateCompleted, v1.TaskStateCancelled, v1.TaskStateFailed, query.AfterTaskID}
	if query.Kind != "" {
		where, args = append(where, "c.kind = ?"), append(args, query.Kind)
	} else {
		where = append(where, "c.kind IN (?, ?, ?)")
		args = append(args, models.TaskCompletionCriterionCheck, models.TaskCompletionCriterionReview, models.TaskCompletionCriterionApproval)
	}
	switch {
	case query.HoldKind != "":
		where, args = append(where, "c.hold_kind = ?"), append(args, query.HoldKind)
	case query.HeldUnescalated:
		where = append(where, "c.hold_kind <> ''", "c.hold_kind <> ?", "c.escalated_at IS NULL")
		args = append(args, holdProtectedFilesChanged)
	default:
		where = append(where, "c.result <> ?", "c.hold_kind = ''")
		args = append(args, models.TaskCompletionResultPass)
		if query.DueBefore != nil {
			where = append(where, "(c.result_subject_revision = '' OR (c.result_reason = ? AND c.result_first_seen_at <= ?))")
			args = append(args, query.AwaitingReason, query.DueBefore.UTC())
		}
	}
	args = append(args, query.Limit)
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT DISTINCT c.task_id FROM task_completion_criteria c
		JOIN tasks t ON t.id = c.task_id
		WHERE `+strings.Join(where, " AND ")+`
		ORDER BY c.task_id
		LIMIT ?`), args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ids = append(ids, id)
	}
	return ids, rows.Err()
}

const maxReworkKeys = 32

// applyReworkCount counts one rework round per key, or holds the criterion
// when the limit is already used. Counted keys are kept so redelivery of an
// earlier event never counts twice; a key refused at the limit is not kept, so
// the same return counts once the limit is raised.
func applyReworkCount(next *models.TaskCompletionCriterion, change models.TaskCompletionControlChange, now time.Time) {
	if change.Key == "" || slices.Contains(next.ReworkKeys, change.Key) {
		return
	}
	if change.CountReworkLimit > 0 && next.ReworkCount >= change.CountReworkLimit {
		if next.HoldKind == "" {
			next.HoldKind, next.HeldAt, next.EscalatedAt = holdReworkLimitReached, &now, nil
		}
		return
	}
	next.ReworkCount++
	next.ReworkKeys = append(next.ReworkKeys, change.Key)
	if len(next.ReworkKeys) > maxReworkKeys {
		next.ReworkKeys = next.ReworkKeys[len(next.ReworkKeys)-maxReworkKeys:]
	}
}

func applyLimitExtension(next *models.TaskCompletionCriterion, change models.TaskCompletionControlChange) error {
	if next.Kind == models.TaskCompletionCriterionApproval && change.VerificationLimit > 0 {
		return fmt.Errorf("%w: an approval criterion has no verification limit", repoerrors.ErrTaskCompletionCriteriaInvalid)
	}
	if err := validateCompletionLimits(models.TaskCompletionCriterionConfig{
		VerificationLimit: change.VerificationLimit, ReworkLimit: change.ReworkLimit,
	}); err != nil {
		return err
	}
	if change.VerificationLimit > 0 {
		next.Config.VerificationLimit = change.VerificationLimit
	}
	if change.ReworkLimit > 0 {
		next.Config.ReworkLimit = change.ReworkLimit
	}
	if change.LimitsConfirmedBy != "" {
		next.Config.LimitsConfirmedBy = change.LimitsConfirmedBy
	}
	// Only raising the limit that caused the hold above what is used releases it.
	releases := (next.HoldKind == holdVerificationLimitReached && change.VerificationLimit > next.AttemptCount) ||
		(next.HoldKind == holdReworkLimitReached && change.ReworkLimit > next.ReworkCount)
	if releases {
		next.HoldKind, next.HeldAt, next.EscalatedAt = "", nil, nil
	}
	return nil
}

// ListHeldCompletionCriteria returns the criteria of open, unarchived tasks in
// a workspace that wait for a person: held criteria and approvals that ask a
// person, oldest first.
func (r *Repository) ListHeldCompletionCriteria(ctx context.Context, workspaceID string) ([]models.HeldTaskCompletionCriterion, error) {
	columns := strings.Split(completionCriterionColumns, ",")
	for index, column := range columns {
		columns[index] = "c." + strings.TrimSpace(column)
	}
	rows, err := r.ro.QueryContext(ctx, r.ro.Rebind(`
		SELECT t.id, t.title, `+strings.Join(columns, ", ")+`
		FROM task_completion_criteria c
		JOIN tasks t ON t.id = c.task_id
		WHERE t.workspace_id = ? AND t.archived_at IS NULL AND t.state NOT IN (?, ?, ?)
		  AND (c.hold_kind <> '' OR (c.kind = ? AND c.result_reason = ?))
		ORDER BY COALESCE(c.held_at, c.result_at), t.id, c.criterion_id
		LIMIT 200
	`), workspaceID, v1.TaskStateCompleted, v1.TaskStateCancelled, v1.TaskStateFailed,
		models.TaskCompletionCriterionApproval, models.TaskCompletionReasonAwaitingPerson)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	var held []models.HeldTaskCompletionCriterion
	for rows.Next() {
		var item models.HeldTaskCompletionCriterion
		var taskID, title string
		criterion, err := scanCompletionCriterion(prefixedScanner{rows: rows, prefix: []any{&taskID, &title}})
		if err != nil {
			return nil, err
		}
		item.TaskID, item.TaskTitle, item.Criterion = taskID, title, criterion
		held = append(held, item)
	}
	return held, rows.Err()
}

// prefixedScanner scans leading columns into prefix before the criterion.
type prefixedScanner struct {
	rows   *sql.Rows
	prefix []any
}

func (s prefixedScanner) Scan(dest ...interface{}) error {
	return s.rows.Scan(append(append([]any{}, s.prefix...), dest...)...)
}
