package completiongates

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

func personContext() context.Context {
	return authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1"})
}

func newPersonService(t *testing.T, current string, criteria ...models.TaskCompletionCriterion) (*Service, *fakeGates, *fakeNavigator, *payloadRuns) {
	t.Helper()
	gates, runs := newFakeGates(), &payloadRuns{}
	gates.setCriteria("task-1", criteria...)
	svc := NewService(Dependencies{
		Gates: gates, PRs: newFakePRs(), Tasks: fakeTasks{assignee: "worker"}, Reviews: &fakeReviews{},
		Settings: &fakeSettings{}, Agents: fakeAgents{ceo: "ceo"}, Runs: runs, Held: fakeHeld{},
		Logger: newTestLogger(t),
	})
	navigator := &fakeNavigator{placement: officePlacement(current)}
	svc.SetNavigator(navigator)
	return svc, gates, navigator, runs
}

func heldReview(kind string) models.TaskCompletionCriterion {
	review := reviewCriterion()
	review.Result, review.ResultSubjectRevision, review.HoldKind = models.TaskCompletionResultPending, "pr-1@h1", kind
	return review
}

func personAction(reason string) PersonAction {
	return PersonAction{ExpectedCriterionRevision: 1, ExpectedSubjectRevision: "pr-1@h1", Reason: reason}
}

// @covers AC-OFFICE-COMPLETION-GATES-008.9
func TestAcceptVerifiesOnlyTheHeldReviewAndNeverMovesTheTask(t *testing.T) {
	svc, gates, navigator, runs := newPersonService(t, "review", heldReview(HoldNoReviewerAvailable))
	if _, err := svc.Accept(personContext(), "task-1", "review", personAction(" ")); !errors.Is(err, ErrReasonRequired) {
		t.Fatalf("accept without a reason err = %v", err)
	}
	if _, err := svc.Accept(context.Background(), "task-1", "review", personAction("read the diff")); !errors.Is(err, ErrHumanRequired) {
		t.Fatalf("accept without a person err = %v", err)
	}
	if _, err := svc.Accept(personContext(), "task-1", "review", personAction("read the diff")); err != nil {
		t.Fatal(err)
	}
	last := gates.controls[len(gates.controls)-1]
	if last.Action != models.TaskCompletionControlAccept || last.SubjectRevision != "pr-1@h1" || last.Reason != "read the diff" ||
		last.ActorKind != "human" || last.ActorID != "user-1" {
		t.Fatalf("control = %+v", last)
	}
	if len(navigator.moves) != 0 || len(runs.agents) != 0 {
		t.Fatalf("accept moved=%v woke=%v", navigator.moves, runs.agents)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-004.6
func TestReviewSendBackRoutesLikeASeatRejection(t *testing.T) {
	passed := reviewCriterion()
	passed.Result, passed.ResultSubjectRevision = models.TaskCompletionResultPass, "pr-1@h1"
	svc, gates, navigator, runs := newPersonService(t, "review", passed)
	if _, err := svc.SendBack(personContext(), "task-1", "review", personAction("misses the error path")); err != nil {
		t.Fatal(err)
	}
	if got := gates.snapshots["task-1"].Criteria[0]; got.ReworkCount != 1 || got.Result != models.TaskCompletionResultFail {
		t.Fatalf("review = %+v", got)
	}
	if len(navigator.moves) != 1 || navigator.moves[0] != "work" || len(runs.agents) != 0 {
		t.Fatalf("moves=%v runs=%v", navigator.moves, runs.agents)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-004.6
func TestReviewSendBackInTheWorkStageWakesTheWorker(t *testing.T) {
	svc, _, navigator, runs := newPersonService(t, "work", reviewCriterion())
	if _, err := svc.SendBack(personContext(), "task-1", "review", personAction("misses the error path")); err != nil {
		t.Fatal(err)
	}
	if len(navigator.moves) != 0 || len(runs.agents) != 1 || runs.agents[0] != "worker" {
		t.Fatalf("moves=%v runs=%v", navigator.moves, runs.agents)
	}
}
