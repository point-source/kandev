package completiongates

import (
	"context"
	"strconv"
	"sync"

	"github.com/kandev/kandev/internal/task/models"
)

// TransitionReader reads a task's step-transition ledger, newest first.
type TransitionReader interface {
	ListTaskStepTransitions(ctx context.Context, taskID string, limit int, cursor string) ([]models.StepTransition, string, error)
}

// maxTrackedTransitions bounds the routed-transition memory; past it the
// memory restarts, and keyed rework counting keeps a repeat route harmless.
const maxTrackedTransitions = 10000

// transitionTracker remembers the newest step transition routed per task so
// the same move, seen through task.moved and task.updated, routes once.
type transitionTracker struct {
	mu   sync.Mutex
	last map[string]int64
}

func newTransitionTracker() *transitionTracker {
	return &transitionTracker{last: map[string]int64{}}
}

// claim reports whether transitionID is newer than any routed for the task.
func (t *transitionTracker) claim(taskID string, transitionID int64) bool {
	t.mu.Lock()
	defer t.mu.Unlock()
	if transitionID <= t.last[taskID] {
		return false
	}
	if len(t.last) >= maxTrackedTransitions {
		t.last = map[string]int64{}
	}
	t.last[taskID] = transitionID
	return true
}

func transitionID(value any) int64 {
	switch typed := value.(type) {
	case float64:
		return int64(typed)
	case int64:
		return typed
	case string:
		parsed, _ := strconv.ParseInt(typed, 10, 64)
		return parsed
	}
	return 0
}

// handleTaskMoved routes a move published by the task service.
func (s *Service) handleTaskMoved(ctx context.Context, payload map[string]any) {
	taskID, _ := payload["task_id"].(string)
	fromStepID, _ := payload["from_step_id"].(string)
	id := transitionID(payload["step_transition_id"])
	if taskID == "" || fromStepID == "" || (id > 0 && !s.transitions.claim(taskID, id)) {
		return
	}
	s.Router.OnTaskMoved(ctx, taskID, fromStepID, transitionKey(payload["step_transition_id"]))
}

// handleTaskUpdated routes workflow-engine moves, which publish task.updated
// rather than task.moved. The task's newest ledger row names the source step.
func (s *Service) handleTaskUpdated(ctx context.Context, payload map[string]any) {
	office, _ := payload["is_from_office"].(bool)
	taskID, _ := payload["task_id"].(string)
	id := transitionID(payload["step_transition_id"])
	if !office || taskID == "" || id <= 0 || s.ledger == nil || !s.transitions.claim(taskID, id) {
		return
	}
	rows, _, err := s.ledger.ListTaskStepTransitions(ctx, taskID, 1, "")
	if err != nil || len(rows) == 0 || rows[0].ID != id {
		return
	}
	from, to := rows[0].FromWorkflowStepID, rows[0].ToWorkflowStepID
	if from == nil || to == nil || *from == "" || *from == *to {
		return
	}
	s.Router.OnTaskMoved(ctx, taskID, *from, strconv.FormatInt(id, 10))
}
