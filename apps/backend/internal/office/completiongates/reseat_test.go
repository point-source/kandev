package completiongates

import (
	"context"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeSeater struct {
	seat  bool
	tasks []string
}

func (f *fakeSeater) SeatReviewer(_ context.Context, taskID string) (bool, error) {
	f.tasks = append(f.tasks, taskID)
	return f.seat, nil
}

func newReseatService(t *testing.T, cause string, seater *fakeSeater) (*Service, *fakeGates) {
	t.Helper()
	gates := newFakeGates()
	held := reviewCriterion()
	held.HoldKind, held.ResultDetail = HoldNoReviewerAvailable, []string{cause}
	gates.setCriteria("task-1", held)
	gates.unmet = []string{"task-1"}
	svc := NewService(Dependencies{
		Gates: gates, PRs: newFakePRs(), Tasks: fakeTasks{assignee: "worker"}, Reviews: &fakeReviews{},
		Settings: &fakeSettings{settings: officemodels.CompletionGateSettings{}}, Agents: fakeAgents{ceo: "ceo"},
		Runs: &payloadRuns{}, Held: fakeHeld{}, Seater: seater, Logger: newTestLogger(t),
	})
	return svc, gates
}

// @covers AC-OFFICE-COMPLETION-GATES-009.4
func TestASeatThatBecomesFillableClearsTheHold(t *testing.T) {
	seater := &fakeSeater{seat: true}
	svc, gates := newReseatService(t, CauseAssigneeOnly, seater)
	svc.sweepReviewerSeats(context.Background())
	if len(seater.tasks) != 1 || reviewState(gates).HoldKind != "" {
		t.Fatalf("seated=%v review=%+v", seater.tasks, reviewState(gates))
	}
	if query := gates.queries[0]; query.HoldKind != HoldNoReviewerAvailable || query.Kind != models.TaskCompletionCriterionReview {
		t.Fatalf("sweep query = %+v", query)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-009.4
func TestAnUnfillableSeatKeepsTheHold(t *testing.T) {
	seater := &fakeSeater{seat: false}
	svc, gates := newReseatService(t, CauseNoReviewerRole, seater)
	svc.sweepReviewerSeats(context.Background())
	if len(seater.tasks) != 1 || reviewState(gates).HoldKind != HoldNoReviewerAvailable {
		t.Fatalf("seated=%v review=%+v", seater.tasks, reviewState(gates))
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-009.4
func TestAnAuthorProviderHoldIsNotReseated(t *testing.T) {
	seater := &fakeSeater{seat: true}
	svc, gates := newReseatService(t, CauseAuthorProviderOnly, seater)
	svc.sweepReviewerSeats(context.Background())
	if len(seater.tasks) != 0 || reviewState(gates).HoldKind != HoldNoReviewerAvailable {
		t.Fatalf("seated=%v review=%+v", seater.tasks, reviewState(gates))
	}
}
