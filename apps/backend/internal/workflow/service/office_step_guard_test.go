package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-OFFICE-WORKFLOW-SELECT-006.4, AC-OFFICE-WORKFLOW-SELECT-003.4
func TestSingleStepWritesRefusedForGuardedOfficeWorkflows(t *testing.T) {
	svc, _, provider := setupTestServiceWithProvider(t)
	provider.addWorkflow("wf-selected", "ws-1", "Selected copy")
	provider.addWorkflow("wf-free", "ws-1", "Kanban")
	createStep(t, svc, seatStepFixture("wf-selected", "step-review"))
	ctx := context.Background()
	svc.SetOfficeStepWritePolicy(func(_ context.Context, workflowID string) (string, error) {
		if workflowID == "wf-selected" {
			return models.OfficeProblemSelectedRequiresDefinitionSave, nil
		}
		return "", nil
	})

	assertGuarded := func(t *testing.T, err error) {
		t.Helper()
		refusal, ok := AsOfficeWorkflowRefusal(err)
		require.True(t, ok, "err = %v", err)
		assert.True(t, refusal.Conflict)
		assert.Equal(t, models.OfficeProblemSelectedRequiresDefinitionSave, refusal.Problems[0].Code)
	}
	assertGuarded(t, svc.CreateStep(ctx, &models.WorkflowStep{WorkflowID: "wf-selected", Name: "New", Position: 2}))
	stored, err := svc.GetStep(ctx, "step-review")
	require.NoError(t, err)
	stored.Name = "Renamed"
	assertGuarded(t, svc.UpdateStep(ctx, stored))
	assertGuarded(t, svc.DeleteStep(ctx, "step-review"))
	assertGuarded(t, svc.ReorderSteps(ctx, "wf-selected", []string{"step-review"}))
	assertGuarded(t, svc.CreateStepsFromTemplate(ctx, "wf-selected", "kanban"))

	require.NoError(t, svc.CreateStep(ctx, &models.WorkflowStep{WorkflowID: "wf-free", Name: "Todo", Position: 0}))

	// The Office > Routing seat control keeps working on guarded workflows.
	_, err = svc.SetSeatAuthorExclusion(ctx, "ws-1", []SeatAuthorExclusionChange{
		{StepID: "step-review", Role: "reviewer", Enabled: true},
	})
	require.NoError(t, err)
}
