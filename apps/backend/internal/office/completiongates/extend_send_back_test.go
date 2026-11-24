package completiongates

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// A send back refused at the rework limit is applied, with its reason, once a
// person raises the limit.
//
// @covers AC-OFFICE-COMPLETION-GATES-004.6, AC-OFFICE-COMPLETION-GATES-007.4
func TestRaisingTheLimitResumesASendBackHeldAtTheLimit(t *testing.T) {
	for _, current := range []string{"review", "work"} {
		t.Run(current, func(t *testing.T) {
			passed := reviewCriterion()
			passed.Result, passed.ResultSubjectRevision, passed.ReworkCount = models.TaskCompletionResultPass, "pr-1@h1", 1
			svc, gates, navigator, runs := newPersonService(t, current, passed)
			svc.Router.SetLimitResolver(func(context.Context, string, models.TaskCompletionCriterion) (int, int) { return 3, 1 })
			if _, err := svc.SendBack(personContext(), "task-1", "review", personAction("misses the error path")); err != nil {
				t.Fatal(err)
			}
			if got := gates.snapshots["task-1"].Criteria[0]; got.HoldKind != HoldReworkLimitReached || len(navigator.moves) != 0 {
				t.Fatalf("at the limit: review=%+v moves=%v", got, navigator.moves)
			}
			escalations := len(runs.agents)
			svc.Router.SetLimitResolver(func(context.Context, string, models.TaskCompletionCriterion) (int, int) { return 3, 2 })
			if _, err := svc.ExtendAsPerson(personContext(), "task-1", "review", ExtendRequest{ExpectedCriterionRevision: 1, ReworkLimit: 2, Reason: "one more"}); err != nil {
				t.Fatal(err)
			}
			if got := gates.snapshots["task-1"].Criteria[0]; got.ReworkCount != 2 || got.HoldKind != "" {
				t.Fatalf("after the extension: review=%+v", got)
			}
			if current == "review" {
				if len(navigator.moves) != 1 || navigator.moves[0] != "work" {
					t.Fatalf("moves = %v", navigator.moves)
				}
				return
			}
			if len(runs.agents) != escalations+1 || runs.agents[len(runs.agents)-1] != "worker" {
				t.Fatalf("worker not woken: %v", runs.agents)
			}
		})
	}
}
