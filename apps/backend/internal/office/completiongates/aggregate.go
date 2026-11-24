package completiongates

import (
	"slices"
	"time"

	"github.com/bmatcuk/doublestar/v4"
	"github.com/kandev/kandev/internal/task/models"
)

// ChecksMissingGrace is how long Kandev waits for checks to appear on a new
// head before recording checks_missing.
const ChecksMissingGrace = 10 * time.Minute

const (
	checkStateFailure = "failure"
	checkStatePending = "pending"
	checkStateSuccess = "success"
)

// ciInput is everything one CI evaluation needs, already read from GitHub.
type ciInput struct {
	criterion   models.TaskCompletionCriterion
	revision    string
	checks      []CheckObservation
	files       []FilesObservation
	firstSeenAt *time.Time
	now         time.Time
}

// ciOutcome is the result to record for one CI evaluation.
type ciOutcome struct {
	result string
	reason string
	detail []string
	hold   string
}

// aggregateCI applies the CI rules across every linked PR, first match wins.
func aggregateCI(input ciInput) ciOutcome {
	// A protected change needs a person before anything else, so a failing
	// check never sends it back to the worker unseen.
	allowed := input.criterion.ProtectedAllowedRevision != "" && input.criterion.ProtectedAllowedRevision == input.revision
	if matched := protectedMatches(input.criterion.Config.ProtectedPaths, input.files); len(matched) > 0 && !allowed {
		return ciOutcome{result: models.TaskCompletionResultPending, reason: HoldProtectedFilesChanged, detail: matched, hold: HoldProtectedFilesChanged}
	}
	if failing := failingCheckNames(input.checks); len(failing) > 0 {
		return ciOutcome{result: models.TaskCompletionResultFail, reason: ReasonChecksFailed, detail: failing}
	}
	// Running checks decide nothing yet, so an incomplete file list waits for
	// them instead of spending an attempt.
	if anyCheckState(input.checks, checkStatePending) {
		return ciOutcome{result: models.TaskCompletionResultPending}
	}
	if len(input.criterion.Config.ProtectedPaths) > 0 && !allFilesComplete(input.files) {
		return ciOutcome{result: models.TaskCompletionResultCannotEvaluate, reason: ReasonChangedFilesIncomplete}
	}
	if anyChecksMissing(input.checks) {
		if input.firstSeenAt != nil && input.now.Sub(*input.firstSeenAt) >= ChecksMissingGrace {
			return ciOutcome{result: models.TaskCompletionResultCannotEvaluate, reason: ReasonChecksMissing}
		}
		return ciOutcome{result: models.TaskCompletionResultPending, reason: ReasonAwaitingChecks}
	}
	return ciOutcome{result: models.TaskCompletionResultPass}
}

func failingCheckNames(checks []CheckObservation) []string {
	var names []string
	for _, check := range checks {
		if check.State == checkStateFailure {
			names = append(names, check.FailingNames...)
		}
	}
	slices.Sort(names)
	return slices.Compact(names)
}

func anyCheckState(checks []CheckObservation, state string) bool {
	return slices.ContainsFunc(checks, func(check CheckObservation) bool { return check.State == state })
}

// anyChecksMissing reports a PR with no counted selected check and no active
// workflow run: nothing ran, or everything was skipped or cancelled.
func anyChecksMissing(checks []CheckObservation) bool {
	return slices.ContainsFunc(checks, func(check CheckObservation) bool {
		return check.State != checkStateSuccess && !check.ActiveWorkflowRuns
	})
}

func allFilesComplete(files []FilesObservation) bool {
	return !slices.ContainsFunc(files, func(file FilesObservation) bool { return !file.Complete })
}

func protectedMatches(patterns []string, files []FilesObservation) []string {
	if len(patterns) == 0 {
		return nil
	}
	var matched []string
	for _, observation := range files {
		for _, path := range observation.Paths {
			if matchesAny(patterns, path) {
				matched = append(matched, path)
			}
		}
	}
	slices.Sort(matched)
	return slices.Compact(matched)
}

func matchesAny(patterns []string, path string) bool {
	for _, pattern := range patterns {
		if ok, err := doublestar.Match(pattern, path); err == nil && ok {
			return true
		}
	}
	return false
}
