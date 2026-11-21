// Package completiongates verifies Office completion criteria that Kandev
// checks itself (REQ-OFFICE-COMPLETION-GATES) and routes their results. It
// writes to the task-owned gate only through task-service commands.
package completiongates

import (
	"context"

	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/task/models"
)

// Result reasons recorded by the verifiers.
const (
	ReasonChecksFailed           = "checks_failed"
	ReasonChecksMissing          = "checks_missing"
	ReasonAwaitingChecks         = "awaiting_checks"
	ReasonChangedFilesIncomplete = "changed_files_incomplete"
	ReasonReviewApproved         = "review_approved"
	ReasonReviewRejected         = "review_rejected"
)

// NoticeSelfApprovalIgnored tells an assignee its own approval does not count.
const NoticeSelfApprovalIgnored = "review_self_approval_ignored"

// Hold reasons.
const (
	HoldProtectedPathChanged = "protected_path_changed"
	HoldAttemptCapReached    = "attempt_cap_reached"
	HoldReworkCapReached     = "rework_cap_reached"
)

// Verifier identities recorded as proof provenance.
const (
	VerifierKindCI      = "kandev_ci"
	VerifierKindReviews = "review_seats"
)

// GateService is the task-service surface the verifiers use.
type GateService interface {
	GetTaskCompletionGate(ctx context.Context, taskID string) (*models.TaskCompletionGateSnapshot, error)
	RecordTaskCompletionEvaluation(ctx context.Context, change models.TaskCompletionEvaluationChange) (*models.TaskCompletionEvaluationOutcome, error)
	ApplyTaskCompletionControl(ctx context.Context, change models.TaskCompletionControlChange) (*models.TaskCompletionControlOutcome, error)
	ListTaskIDsWithUnmetCompletionCriteria(ctx context.Context, query models.TaskCompletionSweepQuery) ([]string, error)
	AuthorizeTaskScope(ctx context.Context, taskID string, scope authz.Scope) error
	AuthorizeWorkspaceScope(ctx context.Context, workspaceID string, scope authz.Scope) error
}

// PRLink is one linked, non-detached pull request of a task.
type PRLink struct {
	ID      string
	Owner   string
	Repo    string
	Number  int
	HeadSHA string
	State   string
}

// CheckObservation is the selected-check state of one PR at its head.
type CheckObservation struct {
	HeadSHA            string
	State              string
	Observed           int
	Skipped            int
	FailingNames       []string
	ActiveWorkflowRuns bool
}

// FilesObservation lists the paths a PR touches at one head.
type FilesObservation struct {
	HeadSHA  string
	Paths    []string
	Complete bool
}

// PRSource reads linked PRs and their live check and file state.
type PRSource interface {
	LinkedPRs(ctx context.Context, taskID string) ([]PRLink, error)
	ObserveChecks(ctx context.Context, taskID string, link PRLink) (CheckObservation, error)
	ChangedFiles(ctx context.Context, taskID string, link PRLink) (FilesObservation, error)
}

// HeadsRevision returns the canonical heads revision of the links.
func HeadsRevision(links []PRLink) string {
	heads := make([]models.TaskPRHead, len(links))
	for index, link := range links {
		heads[index] = models.TaskPRHead{ID: link.ID, HeadSHA: link.HeadSHA}
	}
	return models.TaskPRHeadsRevision(heads)
}

// EvaluationPolicy decides counters and holds before a result is recorded and
// reacts after it is recorded. Result routing implements it.
type EvaluationPolicy interface {
	Prepare(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion, change *models.TaskCompletionEvaluationChange)
	AfterRecord(ctx context.Context, taskID string, outcome *models.TaskCompletionEvaluationOutcome)
}

func findCriterionOfKind(snapshot *models.TaskCompletionGateSnapshot, kind string) (models.TaskCompletionCriterion, bool) {
	if snapshot == nil {
		return models.TaskCompletionCriterion{}, false
	}
	for _, criterion := range snapshot.Criteria {
		if criterion.Kind == kind {
			return criterion, true
		}
	}
	return models.TaskCompletionCriterion{}, false
}
