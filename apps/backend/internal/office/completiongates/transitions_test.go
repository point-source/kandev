package completiongates

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type fakeLedger struct{ rows []models.StepTransition }

func (f *fakeLedger) ListTaskStepTransitions(context.Context, string, int, string) ([]models.StepTransition, string, error) {
	return f.rows, "", nil
}

func ledgerRow(id int64, from, to string) models.StepTransition {
	return models.StepTransition{ID: id, FromWorkflowStepID: &from, ToWorkflowStepID: &to}
}

func engineMoveService(t *testing.T, ledger *fakeLedger) (*Service, *fakeGates, *fakeNavigator) {
	t.Helper()
	gates, prs := newFakeGates(), newFakePRs()
	gates.setCriteria("task-1", failedCI("pr-1@h2"))
	prs.links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h2"}}
	svc := NewService(Dependencies{
		Gates: gates, PRs: prs, Tasks: fakeTasks{assignee: "worker"}, Reviews: &fakeReviews{},
		Settings: &fakeSettings{}, Ledger: ledger, Logger: newTestLogger(t),
	})
	navigator := &fakeNavigator{placement: officePlacement("review")}
	svc.SetNavigator(navigator)
	return svc, gates, navigator
}

func TestEngineMoveOutOfWorkWithFailingCIIsReturned(t *testing.T) {
	svc, gates, navigator := engineMoveService(t, &fakeLedger{rows: []models.StepTransition{ledgerRow(7, "work", "review")}})
	payload := map[string]any{"task_id": "task-1", "is_from_office": true, "step_transition_id": float64(7)}
	svc.handleTaskUpdated(context.Background(), payload)
	svc.handleTaskUpdated(context.Background(), payload)
	svc.handleTaskMoved(context.Background(), map[string]any{"task_id": "task-1", "from_step_id": "work", "step_transition_id": float64(7)})
	if len(navigator.moves) != 1 || navigator.moves[0] != "work" || gates.snapshots["task-1"].Criteria[0].ReworkCount != 1 {
		t.Fatalf("moves=%v rework=%d", navigator.moves, gates.snapshots["task-1"].Criteria[0].ReworkCount)
	}
}

func TestTaskUpdatesWithoutAStepChangeAreIgnored(t *testing.T) {
	ledger := &fakeLedger{rows: []models.StepTransition{ledgerRow(8, "work", "review")}}
	svc, _, navigator := engineMoveService(t, ledger)
	svc.handleTaskUpdated(context.Background(), map[string]any{"task_id": "task-1", "is_from_office": false, "step_transition_id": float64(8)})
	svc.handleTaskUpdated(context.Background(), map[string]any{"task_id": "task-1", "is_from_office": true})
	svc.handleTaskUpdated(context.Background(), map[string]any{"task_id": "task-1", "is_from_office": true, "step_transition_id": float64(9)})
	if len(navigator.moves) != 0 {
		t.Fatalf("moves = %v", navigator.moves)
	}
}
