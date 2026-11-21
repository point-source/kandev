package completiongates

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

func heldProtectedCI() models.TaskCompletionCriterion {
	criterion := ciCriterion("tests/**")
	criterion.Result, criterion.ResultSubjectRevision = models.TaskCompletionResultPending, "pr-1@h1"
	criterion.HeldReason = HoldProtectedPathChanged
	return criterion
}

type payloadRuns struct {
	spyRuns
	reasons, payloads []string
}

func (p *payloadRuns) QueueRun(ctx context.Context, agentID, reason, payload, key string) error {
	p.reasons = append(p.reasons, reason)
	p.payloads = append(p.payloads, payload)
	return p.spyRuns.QueueRun(ctx, agentID, reason, payload, key)
}

func newSendBackService(t *testing.T, current string) (*Service, *fakeGates, *fakeNavigator, *payloadRuns) {
	t.Helper()
	gates, runs := newFakeGates(), &payloadRuns{}
	gates.setCriteria("task-1", heldProtectedCI())
	svc := NewService(Dependencies{
		Gates: gates, PRs: newFakePRs(), Tasks: fakeTasks{assignee: "worker"}, Reviews: &fakeReviews{},
		Settings: &fakeSettings{}, Agents: fakeAgents{ceo: "ceo"}, Runs: runs, Held: fakeHeld{},
		Logger: newTestLogger(t),
	})
	navigator := &fakeNavigator{placement: officePlacement(current)}
	svc.SetNavigator(navigator)
	return svc, gates, navigator, runs
}

func sendBack(t *testing.T, svc *Service) {
	t.Helper()
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1"})
	request := ProtectedPathsAction{Action: ProtectedPathsSendBack, ExpectedCriterionRevision: 1,
		ExpectedSubjectRevision: "pr-1@h1", Reason: "tests were weakened"}
	if _, err := svc.DecideProtectedPaths(ctx, "task-1", "ci", request); err != nil {
		t.Fatal(err)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-003.4
func TestSendBackInTheWorkStageCountsAndWakesTheAssignee(t *testing.T) {
	svc, gates, navigator, runs := newSendBackService(t, "work")
	sendBack(t, svc)
	if got := gates.snapshots["task-1"].Criteria[0].ReworkCount; got != 1 || len(navigator.moves) != 0 {
		t.Fatalf("rework=%d moves=%v", got, navigator.moves)
	}
	if len(runs.agents) != 1 || runs.agents[0] != "worker" || runs.reasons[0] != RunReasonChangesRequested {
		t.Fatalf("runs = %+v", runs)
	}
	var payload map[string]string
	if err := json.Unmarshal([]byte(runs.payloads[0]), &payload); err != nil ||
		payload["task_id"] != "task-1" || payload["decision_comment"] != "tests were weakened" {
		t.Fatalf("payload = %s (%v)", runs.payloads[0], err)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-003.4
func TestSendBackFromALaterStageMovesWithoutASecondWake(t *testing.T) {
	svc, gates, navigator, runs := newSendBackService(t, "approval")
	sendBack(t, svc)
	if got := gates.snapshots["task-1"].Criteria[0].ReworkCount; got != 1 || len(navigator.moves) != 1 || navigator.moves[0] != "work" {
		t.Fatalf("rework=%d moves=%v", got, navigator.moves)
	}
	if len(runs.agents) != 0 {
		t.Fatalf("runs = %v", runs.agents)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-003.4
func TestCIVerifierKeepsASendBackUntilTheHeadsMove(t *testing.T) {
	verifier, gates, prs := setupCI(t, "tests/**")
	sentBack := ciCriterion("tests/**")
	sentBack.Result, sentBack.ResultReason, sentBack.ResultSubjectRevision =
		models.TaskCompletionResultFail, models.TaskCompletionReasonSentBack, "pr-1@h1"
	gates.setCriteria("task-1", sentBack)
	prs.checks["pr-1"] = CheckObservation{HeadSHA: "h1", State: checkStateSuccess, Observed: 1}
	prs.files["pr-1"] = FilesObservation{HeadSHA: "h1", Paths: []string{"tests/a_test.go"}, Complete: true}
	if err := verifier.Evaluate(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	if records := gates.records(); len(records) != 0 {
		t.Fatalf("re-evaluated a sent-back head: %+v", records)
	}
	prs.links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h2"}}
	prs.checks["pr-1"] = CheckObservation{HeadSHA: "h2", State: checkStateSuccess, Observed: 1}
	prs.files["pr-1"] = FilesObservation{HeadSHA: "h2", Paths: []string{"src/a.go"}, Complete: true}
	if err := verifier.Evaluate(context.Background(), "task-1"); err != nil {
		t.Fatal(err)
	}
	if records := gates.records(); len(records) != 1 || records[0].SubjectRevision != "pr-1@h2" {
		t.Fatalf("records at new head = %+v", records)
	}
}
