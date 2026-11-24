package completiongates

import (
	"slices"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/task/models"
)

func ciCriterion(protected ...string) models.TaskCompletionCriterion {
	return models.TaskCompletionCriterion{
		ID: "ci", Kind: models.TaskCompletionCriterionCheck, CriterionRevision: 1,
		Config: models.TaskCompletionCriterionConfig{ProtectedPaths: protected},
	}
}

func success() CheckObservation { return CheckObservation{State: checkStateSuccess, Observed: 2} }

// @covers AC-OFFICE-COMPLETION-GATES-002.1, AC-OFFICE-COMPLETION-GATES-002.2, AC-OFFICE-COMPLETION-GATES-002.3, AC-OFFICE-COMPLETION-GATES-002.4
func TestAggregateCI(t *testing.T) {
	now := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	recent, old := now.Add(-time.Minute), now.Add(-ChecksMissingGrace)
	cases := map[string]struct {
		input ciInput
		want  ciOutcome
	}{
		"all pass": {ciInput{criterion: ciCriterion(), checks: []CheckObservation{success(), success()}},
			ciOutcome{result: models.TaskCompletionResultPass}},
		"one PR fails": {ciInput{criterion: ciCriterion(), checks: []CheckObservation{success(), {State: checkStateFailure, FailingNames: []string{"lint"}}}},
			ciOutcome{result: models.TaskCompletionResultFail, reason: ReasonChecksFailed, detail: []string{"lint"}}},
		"running": {ciInput{criterion: ciCriterion(), checks: []CheckObservation{{State: checkStatePending}}},
			ciOutcome{result: models.TaskCompletionResultPending}},
		"no checks within grace": {ciInput{criterion: ciCriterion(), checks: []CheckObservation{{}}, firstSeenAt: &recent, now: now},
			ciOutcome{result: models.TaskCompletionResultPending, reason: ReasonAwaitingChecks}},
		"no checks after grace": {ciInput{criterion: ciCriterion(), checks: []CheckObservation{{Observed: 3, Skipped: 3}}, firstSeenAt: &old, now: now},
			ciOutcome{result: models.TaskCompletionResultCannotEvaluate, reason: ReasonChecksMissing}},
		"running checks wait for an incomplete file list": {ciInput{criterion: ciCriterion("tests/**"), checks: []CheckObservation{{State: checkStatePending}},
			files: []FilesObservation{{Complete: false}}}, ciOutcome{result: models.TaskCompletionResultPending}},
		"active workflow is pending": {ciInput{criterion: ciCriterion(), checks: []CheckObservation{{State: checkStatePending, ActiveWorkflowRuns: true}}, firstSeenAt: &old, now: now},
			ciOutcome{result: models.TaskCompletionResultPending}},
	}
	for name, tc := range cases {
		got := aggregateCI(tc.input)
		if got.result != tc.want.result || got.reason != tc.want.reason || !slices.Equal(got.detail, tc.want.detail) || got.hold != tc.want.hold {
			t.Errorf("%s: got %+v, want %+v", name, got, tc.want)
		}
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-003.1, AC-OFFICE-COMPLETION-GATES-003.3, AC-OFFICE-COMPLETION-GATES-003.5
func TestAggregateCIProtectedPaths(t *testing.T) {
	files := []FilesObservation{{Paths: []string{"src/a.go", "tests/webhook_test.go"}, Complete: true}}
	held := aggregateCI(ciInput{criterion: ciCriterion("tests/**"), revision: "pr@h1", checks: []CheckObservation{success()}, files: files})
	if held.hold != HoldProtectedFilesChanged || !slices.Equal(held.detail, []string{"tests/webhook_test.go"}) {
		t.Fatalf("protected change = %+v", held)
	}
	allowed := ciCriterion("tests/**")
	allowed.ProtectedAllowedRevision = "pr@h1"
	if got := aggregateCI(ciInput{criterion: allowed, revision: "pr@h1", checks: []CheckObservation{success()}, files: files}); got.result != models.TaskCompletionResultPass {
		t.Fatalf("allowed protected change = %+v", got)
	}
	if got := aggregateCI(ciInput{criterion: allowed, revision: "pr@h2", checks: []CheckObservation{success()}, files: files}); got.hold != HoldProtectedFilesChanged {
		t.Fatalf("allowance carried to a new head: %+v", got)
	}
	renamed := []FilesObservation{{Paths: []string{"src/moved.go", ".github/workflows/ci.yml"}, Complete: true}}
	if got := aggregateCI(ciInput{criterion: ciCriterion(".github/**"), checks: []CheckObservation{success()}, files: renamed}); got.hold != HoldProtectedFilesChanged {
		t.Fatalf("rename source not matched: %+v", got)
	}
	failing := []CheckObservation{{State: checkStateFailure, FailingNames: []string{"lint"}}}
	if got := aggregateCI(ciInput{criterion: ciCriterion("tests/**"), checks: failing, files: files}); got.hold != HoldProtectedFilesChanged {
		t.Fatalf("failing checks sent back a protected change without a person: %+v", got)
	}
	partial := []FilesObservation{{Paths: []string{"src/a.go"}, Complete: false}}
	if got := aggregateCI(ciInput{criterion: ciCriterion("tests/**"), checks: []CheckObservation{success()}, files: partial}); got.result != models.TaskCompletionResultCannotEvaluate || got.reason != ReasonChangedFilesIncomplete {
		t.Fatalf("incomplete list = %+v", got)
	}
}
