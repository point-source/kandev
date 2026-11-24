package orchestrator

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
)

// @covers AC-OFFICE-COMPLETION-GATES-007.3
func TestAutoStartAgentCallback_WithholdsAHeldAssignee(t *testing.T) {
	ctx := context.Background()
	repo, svc, _ := setupBusyParentChildrenCompleted(t)
	session, err := repo.GetTaskSession(ctx, busyParentSessionID)
	require.NoError(t, err)
	session.AgentProfileID = "worker"
	session.State = models.TaskSessionStateWaitingForInput
	require.NoError(t, repo.UpdateTaskSession(ctx, session))

	held := true
	var asked []string
	svc.SetAssigneeStartHold(func(_ context.Context, taskID, agentID string) bool {
		asked = append(asked, taskID+"/"+agentID)
		return held
	})
	launches := 0
	cb := &autoStartAgentCallback{svc: svc, launch: func(context.Context, engine.ActionInput) error {
		launches++
		return nil
	}}

	_, err = cb.Execute(ctx, busyParentAutoStartInput())
	require.NoError(t, err)
	require.Zero(t, launches, "a held assignee must not be started")
	require.Equal(t, []string{busyParentID + "/worker"}, asked)

	held = false
	_, err = cb.Execute(ctx, busyParentAutoStartInput())
	require.NoError(t, err)
	require.Equal(t, 1, launches, "an unheld assignee starts normally")
}
