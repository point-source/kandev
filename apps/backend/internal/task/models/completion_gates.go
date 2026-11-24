package models

import (
	"context"
	"sort"
	"strings"
	"time"
)

const (
	TaskCompletionEvidenceTaskRevision = "task_revision"
	TaskCompletionEvidenceExecution    = "execution"
	TaskCompletionEvidenceArtifact     = "artifact_revision"
	TaskCompletionEvidenceGitHubPRHead = "github_pr_head"
	// TaskCompletionEvidenceGitHubPRHeads names the set of heads of every
	// linked, non-detached pull request of the task as one revision. ID is the
	// task ID.
	TaskCompletionEvidenceGitHubPRHeads = "github_pr_heads"
)

// Criterion kinds. Host-verified kinds accept results only from in-process
// verifiers through RecordTaskCompletionEvaluation.
const (
	TaskCompletionCriterionAttested = "attested"
	TaskCompletionCriterionCheck    = "check"
	TaskCompletionCriterionReview   = "review"
	// TaskCompletionCriterionApproval passes only on a person's approval of
	// the live PR heads.
	TaskCompletionCriterionApproval = "approval"
)

// Check runners. A check criterion names the program that decides it.
const (
	// TaskCompletionCheckRunnerGitHub reads CI checks at the PR head from GitHub.
	TaskCompletionCheckRunnerGitHub = "github"
)

// Evaluation results recorded for a criterion.
const (
	TaskCompletionResultPending        = "pending"
	TaskCompletionResultPass           = "pass"
	TaskCompletionResultFail           = "fail"
	TaskCompletionResultCannotEvaluate = "cannot_evaluate"
)

// Verdict sources, strongest first. They are derived from what produced a
// criterion's verdict and never stored.
const (
	TaskCompletionVerdictProgram = "program"
	TaskCompletionVerdictPerson  = "person"
	TaskCompletionVerdictAgent   = "agent"
)

// TaskCompletionVerifierHuman is the verifier kind of a person's decision. It
// matches the "human" actor kind of the gate history.
const TaskCompletionVerifierHuman = "human"

// Gate blocker reasons.
const (
	TaskCompletionBlockerUnverified         = "unverified"
	TaskCompletionBlockerEvidenceStale      = "evidence_stale"
	TaskCompletionBlockerPRMissing          = "pr_missing"
	TaskCompletionBlockerPRMergedUnverified = "pr_merged_unverified"
	TaskCompletionBlockerHeld               = "held"
)

// IsHostVerifiedCriterionKind reports whether only in-process verifiers may
// record results for a criterion kind.
func IsHostVerifiedCriterionKind(kind string) bool {
	return kind == TaskCompletionCriterionCheck || kind == TaskCompletionCriterionReview ||
		kind == TaskCompletionCriterionApproval
}

// NormalizeTaskCompletionCriterionKind maps the legacy empty kind to attested.
func NormalizeTaskCompletionCriterionKind(kind string) string {
	if kind == "" {
		return TaskCompletionCriterionAttested
	}
	return kind
}

// TaskCompletionVerdictSource derives the source of a verified criterion's
// verdict: a person's decision, else a program for checks and an agent for
// every other kind. An unverified criterion has no source.
func TaskCompletionVerdictSource(criterion TaskCompletionCriterion) string {
	switch {
	case criterion.VerifierKind == "":
		return ""
	case criterion.VerifierKind == TaskCompletionVerifierHuman:
		return TaskCompletionVerdictPerson
	case criterion.Kind == TaskCompletionCriterionCheck:
		return TaskCompletionVerdictProgram
	default:
		return TaskCompletionVerdictAgent
	}
}

// TaskCompletionCriterionID returns the canonical ID of a host-verified
// criterion: "<runner>_check" for a check and the kind for a review or an
// approval.
func TaskCompletionCriterionID(kind, runner string) string {
	if kind == TaskCompletionCriterionCheck {
		return runner + "_check"
	}
	return kind
}

// TaskCompletionCriterionConfig carries creator-owned settings. Limits of zero
// mean "not set on the criterion".
type TaskCompletionCriterionConfig struct {
	// Runner names the program that decides a check criterion.
	Runner            string   `json:"runner,omitempty"`
	ProtectedPaths    []string `json:"protected_paths,omitempty"`
	VerificationLimit int      `json:"verification_limit,omitempty"`
	ReworkLimit       int      `json:"rework_limit,omitempty"`
	// ReviewWaitLimit is how many minutes a parked reviewer seat run may wait
	// before the review holds. Zero means wait indefinitely.
	ReviewWaitLimit   int    `json:"review_wait_limit,omitempty"`
	LimitsConfirmedBy string `json:"limits_confirmed_by,omitempty"`
}

// TaskCompletionEvidenceSubject identifies a typed, revisioned source used to
// verify one task completion criterion. ID is scoped to the owning task.
type TaskCompletionEvidenceSubject struct {
	Kind     string `json:"kind"`
	ID       string `json:"id"`
	Revision string `json:"revision,omitempty"`
}

// TaskCompletionEvidence is the current attributed verification for a
// criterion. The subject revision is immutable evidence identity; mutable
// task and pull-request subjects are also checked against their live row at
// the final completion commit.
type TaskCompletionEvidence struct {
	Subject   TaskCompletionEvidenceSubject `json:"subject"`
	Summary   string                        `json:"summary,omitempty"`
	Reference string                        `json:"reference,omitempty"`
}

// TaskCompletionCriterion is one required item in a revisioned task gate.
// CriterionRevision changes only when this criterion's meaning or required
// evidence subject changes, so unrelated edits do not invalidate its proof.
type TaskCompletionCriterion struct {
	ID                string                        `json:"id"`
	Description       string                        `json:"description"`
	EvidenceSubject   TaskCompletionEvidenceSubject `json:"evidence_subject"`
	CriterionRevision int64                         `json:"criterion_revision"`
	VerifiedRevision  int64                         `json:"verified_revision,omitempty"`
	Evidence          *TaskCompletionEvidence       `json:"evidence,omitempty"`
	VerifierKind      string                        `json:"verifier_kind,omitempty"`
	VerifierID        string                        `json:"verifier_id,omitempty"`
	VerifiedAt        *time.Time                    `json:"verified_at,omitempty"`

	Kind   string                        `json:"kind,omitempty"`
	Config TaskCompletionCriterionConfig `json:"config"`
	// VerdictSource is derived on read; see TaskCompletionVerdictSource.
	VerdictSource string `json:"verdict_source,omitempty"`
	OwnerKind     string `json:"owner_kind,omitempty"`
	OwnerID       string `json:"owner_id,omitempty"`

	Result                string     `json:"result,omitempty"`
	ResultReason          string     `json:"result_reason,omitempty"`
	ResultDetail          []string   `json:"result_detail,omitempty"`
	ResultSubjectRevision string     `json:"result_subject_revision,omitempty"`
	ResultFirstSeenAt     *time.Time `json:"result_first_seen_at,omitempty"`
	ResultAt              *time.Time `json:"result_at,omitempty"`
	AttemptCount          int        `json:"attempt_count"`
	ReworkCount           int        `json:"rework_count"`
	HoldKind              string     `json:"hold_kind,omitempty"`
	HeldAt                *time.Time `json:"held_at,omitempty"`

	ProtectedAllowedRevision string `json:"protected_allowed_revision,omitempty"`
	ProtectedAllowedBy       string `json:"protected_allowed_by,omitempty"`
	// ReworkKeys are the identities of recently counted rework rounds.
	ReworkKeys []string `json:"-"`
	// EscalatedAt is set once a hold has been escalated to its owner.
	EscalatedAt *time.Time `json:"escalated_at,omitempty"`
}

// TaskCompletionBlocker is a bounded reason that one required criterion
// currently prevents completion.
type TaskCompletionBlocker struct {
	CriterionID string `json:"criterion_id"`
	Reason      string `json:"reason"`
}

// TaskCompletionGateSnapshot is the canonical task-owned completion view.
type TaskCompletionGateSnapshot struct {
	TaskID      string                    `json:"task_id"`
	WorkspaceID string                    `json:"workspace_id"`
	Revision    int64                     `json:"revision"`
	Criteria    []TaskCompletionCriterion `json:"criteria"`
	Blockers    []TaskCompletionBlocker   `json:"blockers,omitempty"`
	Blocked     bool                      `json:"blocked"`
	// HeadsRevision is the live PR heads revision of a task with host-verified
	// criteria ("<id>@<head>,..."); empty without them or without a PR.
	HeadsRevision string `json:"heads_revision,omitempty"`
	// StateRevision increases on every result, hold or control change.
	StateRevision int64 `json:"state_revision"`
}

// TaskCompletionGateSummaryObservation is the compact, keyed status view of
// one task's gate. It carries counts only, without criterion text or evidence.
type TaskCompletionGateSummaryObservation struct {
	Revision      int64 `json:"revision"`
	CriteriaCount int   `json:"criteria_count"`
	VerifiedCount int   `json:"verified_count"`
	BlockerCount  int   `json:"blocker_count"`
	Blocked       bool  `json:"blocked"`
	// StateRevision increases on every result, hold or control change.
	StateRevision int64 `json:"state_revision,omitempty"`
}

// TaskCompletionGateSummaryBatch includes every requested task that still
// exists, even when its observation is nil because no gate is configured.
// MissingTaskIDs identifies requested tasks that no longer exist.
type TaskCompletionGateSummaryBatch struct {
	ByTaskID       map[string]*TaskCompletionGateSummaryObservation `json:"by_task_id"`
	MissingTaskIDs []string                                         `json:"missing_task_ids"`
}

// TaskCompletionGateHistory is an immutable criteria, evidence, or override
// audit event.
type TaskCompletionGateHistory struct {
	ID          string    `json:"id"`
	TaskID      string    `json:"task_id"`
	WorkspaceID string    `json:"workspace_id"`
	Revision    int64     `json:"revision"`
	Action      string    `json:"action"`
	ActorKind   string    `json:"actor_kind"`
	ActorID     string    `json:"actor_id"`
	Reason      string    `json:"reason,omitempty"`
	Details     string    `json:"details,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
}

// TaskCompletionCriteriaChange is the repository command for replacing the
// current criterion set under an observed set revision.
type TaskCompletionCriteriaChange struct {
	TaskID                      string
	WorkspaceID                 string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	OperationID                 string
	PayloadDigest               string
	ClaimFence                  TaskManagementClaimFence
	Criteria                    []TaskCompletionCriterion
	ActorKind                   string
	ActorID                     string
	HumanConfirmationRevision   int64
	HumanConfirmationReason     string
}

// TaskCompletionEvidenceChange verifies one criterion under both the set and
// criterion revisions observed by the caller.
type TaskCompletionEvidenceChange struct {
	TaskID                      string
	WorkspaceID                 string
	ExpectedTaskResourceVersion string
	ExpectedRevision            int64
	OperationID                 string
	PayloadDigest               string
	ClaimFence                  TaskManagementClaimFence
	CriterionID                 string
	Evidence                    TaskCompletionEvidence
	ActorKind                   string
	ActorID                     string
}

// TaskCompletionEvaluationChange records one in-process verifier result for a
// host-verified criterion. It applies only while the criterion revision and
// the live PR heads still equal the observed values.
type TaskCompletionEvaluationChange struct {
	TaskID            string
	CriterionID       string
	CriterionRevision int64
	SubjectRevision   string
	Result            string
	Reason            string
	Detail            []string
	EvidenceSummary   string
	EvidenceReference string
	VerifierKind      string
	VerifierID        string
	// CountAttempt and CountRework increment the counter when the result
	// changes. Hold sets the hold reason when non-empty.
	CountAttempt bool
	CountRework  bool
	Hold         string
	// ReleaseHold clears the hold when its current reason equals this value.
	ReleaseHold string
}

// TaskCompletionEvaluationOutcome reports whether the evaluation changed the
// stored criterion. A repeat of the same observation returns Changed=false.
type TaskCompletionEvaluationOutcome struct {
	Snapshot  *TaskCompletionGateSnapshot
	Criterion TaskCompletionCriterion
	Changed   bool
}

// Control actions applied by people or the owning system to one criterion.
const (
	TaskCompletionControlReleaseHold    = "release_hold"
	TaskCompletionControlExtendLimits   = "extend_limits"
	TaskCompletionControlAllowProtected = "allow_protected_paths"
	// TaskCompletionControlCountRework counts one rework round per Key and
	// holds the criterion instead when CountReworkLimit rounds were already used.
	TaskCompletionControlCountRework = "count_rework"
	// TaskCompletionControlSendBack is a person's send back at the observed
	// heads: it fails the criterion with the reason, releasing a protected-path
	// hold on a check. A review can be sent back at any time.
	TaskCompletionControlSendBack = "send_back"
	// TaskCompletionControlAccept is a person's acceptance of a held review at
	// the observed heads; it verifies only that criterion.
	TaskCompletionControlAccept = "accept"
	// TaskCompletionControlApprove is a person's approval of an approval
	// criterion that asks them, at the observed heads.
	TaskCompletionControlApprove = "approve"
	// TaskCompletionReasonApproved is the result reason of an approval a
	// person gave; it passes the criterion until the heads move.
	TaskCompletionReasonApproved = "approved"
	// TaskCompletionReasonAwaitingPerson is the pending reason of an approval
	// that asks a person: the task left the work stage and every other
	// criterion passes at the live heads.
	TaskCompletionReasonAwaitingPerson = "awaiting_person"
	// TaskCompletionReasonAwaitingCriteria is the pending reason of an
	// approval not yet asked; its detail names what it waits for.
	TaskCompletionReasonAwaitingCriteria = "awaiting_criteria"
	// TaskCompletionControlHold holds a criterion with HoldKind and Detail.
	TaskCompletionControlHold = "hold"
	// TaskCompletionReasonAccepted is the result reason of an accepted review.
	TaskCompletionReasonAccepted = "accepted"
	// TaskCompletionReasonSentBack is the result reason of a criterion a
	// person sent back; it fails the criterion until the heads move.
	TaskCompletionReasonSentBack = "sent_back"
	// TaskCompletionControlMarkEscalated records that a hold was escalated.
	TaskCompletionControlMarkEscalated = "mark_escalated"
)

// TaskCompletionSweepQuery selects open tasks with host-verified criteria to
// revisit, one page at a time ordered by task ID.
type TaskCompletionSweepQuery struct {
	// Kind limits the criteria to one kind; empty matches every host kind.
	Kind string
	// AfterTaskID continues after the last task ID of the previous page.
	AfterTaskID string
	// DueBefore, when set, keeps only criteria never evaluated and criteria
	// whose AwaitingReason result was first seen at or before it.
	DueBefore      *time.Time
	AwaitingReason string
	// HeldUnescalated selects held criteria not yet escalated instead of
	// unmet, unheld ones. Protected-path holds are excluded.
	HeldUnescalated bool
	// HoldKind selects criteria held with this hold kind, escalated or not.
	HoldKind string
	Limit    int
}

// TaskCompletionControlOutcome reports a control result. Claimed is true when
// count_rework counted a new round or hold set a new hold in this call.
type TaskCompletionControlOutcome struct {
	Snapshot *TaskCompletionGateSnapshot
	Claimed  bool
}

// TaskCompletionControlChange applies one recovery action to a criterion.
type TaskCompletionControlChange struct {
	TaskID            string
	CriterionID       string
	CriterionRevision int64
	Action            string
	// SubjectRevision binds allow_protected_paths, send_back, accept and
	// approve to the observed heads.
	SubjectRevision string
	// HoldKind and Detail apply to hold, which also records SubjectRevision as
	// the heads it holds at when set; release_hold clears only a hold of
	// HoldKind when it is set.
	HoldKind string
	Detail   []string
	// RequireResultReason, when set, applies a hold only while the criterion's
	// result reason still equals it; otherwise the hold changes nothing.
	RequireResultReason string
	// RebindFromSubject, when set, moves an existing hold: it applies only
	// while the criterion still holds HoldKind at this subject, and
	// SubjectRevision must name the live heads.
	RebindFromSubject string
	// VerificationLimit and ReworkLimit are the new limits for
	// extend_limits; zero leaves a limit unchanged.
	VerificationLimit int
	ReworkLimit       int
	LimitsConfirmedBy string
	// Key and CountReworkLimit apply to count_rework. A zero limit means unlimited.
	Key              string
	CountReworkLimit int
	ActorKind        string
	ActorID          string
	Reason           string
}

// CompletionGateCaller identifies an Office run making a gate request. The
// Office runtime adapter sets it from the run token.
type CompletionGateCaller struct {
	AgentProfileID string
	RunID          string
	TaskID         string
}

type completionGateCallerContextKey struct{}

// WithCompletionGateCaller attaches the authenticated Office run caller.
func WithCompletionGateCaller(ctx context.Context, caller CompletionGateCaller) context.Context {
	return context.WithValue(ctx, completionGateCallerContextKey{}, caller)
}

// CompletionGateCallerFromContext reads the Office run caller, if any.
func CompletionGateCallerFromContext(ctx context.Context) (CompletionGateCaller, bool) {
	value, ok := ctx.Value(completionGateCallerContextKey{}).(CompletionGateCaller)
	return value, ok
}

// TaskCompletionMoveOverride authorizes one completion transition. The
// repository checks every observed identity and revision again in the task
// transaction and records the override in the same commit.
type TaskCompletionMoveOverride struct {
	TaskID           string
	WorkspaceID      string
	ExpectedRevision int64
	SourceWorkflowID string
	SourceStepID     string
	TargetWorkflowID string
	TargetStepID     string
	ActorID          string
	Reason           string
}

type completionMoveOverrideContextKey struct{}

// WithTaskCompletionMoveOverride passes a native, user-confirmed override to
// the final task write. Only the task service constructs this value from an
// authenticated human request; plugin task commands do not expose it.
func WithTaskCompletionMoveOverride(ctx context.Context, override TaskCompletionMoveOverride) context.Context {
	return context.WithValue(ctx, completionMoveOverrideContextKey{}, override)
}

// TaskCompletionMoveOverrideFromContext reads the one-move override prepared
// by the native task service.
func TaskCompletionMoveOverrideFromContext(ctx context.Context) (TaskCompletionMoveOverride, bool) {
	value, ok := ctx.Value(completionMoveOverrideContextKey{}).(TaskCompletionMoveOverride)
	return value, ok
}

// TaskPRHead is one linked, non-detached pull request and its head commit.
type TaskPRHead struct {
	ID      string
	HeadSHA string
}

// TaskPRHeadsRevision builds the canonical revision of a task's PR heads:
// "<id>@<head>" entries sorted by ID and joined by commas.
func TaskPRHeadsRevision(heads []TaskPRHead) string {
	sorted := append([]TaskPRHead(nil), heads...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ID < sorted[j].ID })
	parts := make([]string, len(sorted))
	for index, head := range sorted {
		parts[index] = head.ID + "@" + head.HeadSHA
	}
	return strings.Join(parts, ",")
}

// HeldTaskCompletionCriterion is a criterion waiting for a person, held or an
// approval that asks them, with its task identity.
type HeldTaskCompletionCriterion struct {
	TaskID    string                  `json:"task_id"`
	TaskTitle string                  `json:"task_title"`
	Criterion TaskCompletionCriterion `json:"criterion"`
}

// TaskCompletionCriterionRequest is a criterion named by a task creator, before
// the owning system resolves defaults, ownership and validation.
type TaskCompletionCriterionRequest struct {
	Kind              string   `json:"kind"`
	Runner            string   `json:"runner,omitempty"`
	ProtectedPaths    []string `json:"protected_paths,omitempty"`
	VerificationLimit int      `json:"verification_limit,omitempty"`
	ReworkLimit       int      `json:"rework_limit,omitempty"`
	ReviewWaitLimit   int      `json:"review_wait_limit,omitempty"`
}

// Creator kinds for completion criteria resolution.
const (
	CompletionCriteriaCreatorHuman   = "human"
	CompletionCriteriaCreatorAgent   = "agent"
	CompletionCriteriaCreatorRoutine = "routine"
)

// CompletionCriteriaCreator identifies who creates a task.
type CompletionCriteriaCreator struct {
	Kind string
	ID   string
}

// CompletionCriteriaResolution is the input for resolving creation criteria.
type CompletionCriteriaResolution struct {
	WorkspaceID   string
	WorkflowID    string
	IsOffice      bool
	HasRepository bool
	// Requested is nil when the creator named no criteria.
	Requested *[]TaskCompletionCriterionRequest
	Creator   CompletionCriteriaCreator
}
