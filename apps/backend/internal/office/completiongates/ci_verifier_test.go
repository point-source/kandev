package completiongates

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

func newTestLogger(t *testing.T) *logger.Logger {
	t.Helper()
	log, err := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "json"})
	if err != nil {
		t.Fatal(err)
	}
	return log
}

func setupCI(t *testing.T, protected ...string) (*CIVerifier, *fakeGates, *fakePRs) {
	t.Helper()
	gates, prs := newFakeGates(), newFakePRs()
	gates.setCriteria("task-1", ciCriterion(protected...))
	prs.links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h1"}}
	return NewCIVerifier(gates, prs, nil, newTestLogger(t)), gates, prs
}

// @covers AC-OFFICE-COMPLETION-GATES-002.1, AC-OFFICE-COMPLETION-GATES-005.4
func TestCIVerifierRecordsPassPinnedToHeads(t *testing.T) {
	verifier, gates, prs := setupCI(t)
	prs.checks["pr-1"] = CheckObservation{HeadSHA: "h1", State: checkStateSuccess, Observed: 1}
	if err := verifier.Evaluate(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	records := gates.records()
	if len(records) != 1 {
		t.Fatalf("records = %d", len(records))
	}
	got := records[0]
	if got.Result != models.TaskCompletionResultPass || got.SubjectRevision != "pr-1@h1" || got.VerifierKind != VerifierKindCI ||
		got.ReleaseHold != HoldProtectedPathChanged || prs.fileReads != 0 {
		t.Fatalf("record = %+v fileReads=%d", got, prs.fileReads)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-002.5
func TestCIVerifierDropsObservationOfAMovedHead(t *testing.T) {
	verifier, gates, prs := setupCI(t)
	prs.checks["pr-1"] = CheckObservation{HeadSHA: "h2", State: checkStateSuccess, Observed: 1}
	if err := verifier.Evaluate(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	if records := gates.records(); len(records) != 0 {
		t.Fatalf("records for stale head = %+v", records)
	}
}

func TestCIVerifierHoldsProtectedChangeAndSkipsCapHolds(t *testing.T) {
	verifier, gates, prs := setupCI(t, "tests/**")
	prs.checks["pr-1"] = CheckObservation{HeadSHA: "h1", State: checkStateSuccess, Observed: 1}
	prs.files["pr-1"] = FilesObservation{HeadSHA: "h1", Paths: []string{"tests/x_test.go"}, Complete: true}
	if err := verifier.Evaluate(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	if got := gates.records(); len(got) != 1 || got[0].Hold != HoldProtectedPathChanged || got[0].ReleaseHold != "" {
		t.Fatalf("records = %+v", got)
	}
	held := ciCriterion("tests/**")
	held.HeldReason = HoldReworkCapReached
	gates.setCriteria("task-1", held)
	if err := verifier.Evaluate(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	if got := gates.records(); len(got) != 1 {
		t.Fatalf("cap-held criterion was evaluated: %+v", got)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-002.7
func TestCIVerifierEvaluatesOnPRUpdatesAndAtStartup(t *testing.T) {
	gates, prs := newFakeGates(), newFakePRs()
	gates.setCriteria("task-1", ciCriterion())
	gates.setCriteria("task-2", ciCriterion())
	gates.unmet = []string{"task-2"}
	prs.links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h1"}}
	prs.links["task-2"] = []PRLink{{ID: "pr-2", HeadSHA: "h9"}}
	prs.checks["pr-1"] = CheckObservation{HeadSHA: "h1", State: checkStatePending}
	prs.checks["pr-2"] = CheckObservation{HeadSHA: "h9", State: checkStateSuccess, Observed: 1}
	log := newTestLogger(t)
	eventBus := bus.NewMemoryEventBus(log)
	verifier := NewCIVerifier(gates, prs, eventBus, log)
	if err := verifier.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(verifier.Stop)
	if err := eventBus.Publish(context.Background(), events.GitHubTaskPRUpdated,
		bus.NewEvent(events.GitHubTaskPRUpdated, "test", map[string]any{"task_id": "task-1"})); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if seen := recordedTasks(gates.records()); seen["task-1"] && seen["task-2"] {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("records = %+v", gates.records())
}

func recordedTasks(records []models.TaskCompletionEvaluationChange) map[string]bool {
	seen := map[string]bool{}
	for _, record := range records {
		seen[record.TaskID] = true
	}
	return seen
}

func TestPeriodicSweepQueriesOnlyDueCriteriaAcrossPages(t *testing.T) {
	verifier, gates, _ := setupCI(t)
	now := time.Date(2026, 11, 21, 12, 0, 0, 0, time.UTC)
	verifier.now = func() time.Time { return now }
	for index := 0; index < sweepBatchSize+5; index++ {
		gates.unmet = append(gates.unmet, fmt.Sprintf("task-%03d", index))
	}
	scheduled := map[string]bool{}
	visit := func(taskID string) { scheduled[taskID] = true }
	query := models.TaskCompletionSweepQuery{Kind: models.TaskCompletionCriterionCIAtHead, Limit: sweepBatchSize}
	dueBefore := now.Add(-ChecksMissingGrace)
	query.DueBefore, query.AwaitingReason = &dueBefore, ReasonAwaitingChecks
	if err := forEachSweepPage(context.Background(), gates, query, visit); err != nil {
		t.Fatal(err)
	}
	if len(scheduled) != sweepBatchSize+5 {
		t.Fatalf("scheduled %d tasks, want %d", len(scheduled), sweepBatchSize+5)
	}
	verifier.sweep(context.Background(), false)
	last := gates.queries[len(gates.queries)-1]
	if last.DueBefore == nil || !last.DueBefore.Equal(dueBefore) || last.AwaitingReason != ReasonAwaitingChecks {
		t.Fatalf("periodic sweep query = %+v", last)
	}
}
