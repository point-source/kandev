package service

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-OFFICE-WORKFLOW-SELECT-004.2, AC-OFFICE-WORKFLOW-SELECT-004.3
func TestSelectOfficeWorkflowSelectsAndMarksEmptyWorkflow(t *testing.T) {
	f := newOfficeFixture(t)
	ctx := context.Background()
	builtin := f.officeWorkspace(t, "ws-1")
	f.workflow(t, "ws-1", "wf-empty", nil)
	f.validSteps(t, "wf-empty")

	require.NoError(t, f.svc.SelectOfficeWorkflow(ctx, "ws-1", "wf-empty"))
	assert.Equal(t, "wf-empty", f.selected(t, "ws-1"))
	assert.True(t, f.isOffice(t, "wf-empty"), "selection marks the workflow Office")
	assert.True(t, f.isOffice(t, builtin), "previous Office workflow stays Office")

	require.NoError(t, f.svc.SelectOfficeWorkflow(ctx, "ws-1", builtin))
	assert.Equal(t, builtin, f.selected(t, "ws-1"), "the built-in can be selected again")
}

// @covers AC-OFFICE-WORKFLOW-SELECT-005.6, AC-OFFICE-WORKFLOW-SELECT-004.4
func TestSelectOfficeWorkflowRefusesInvalidWorkflowWithoutWriting(t *testing.T) {
	f := newOfficeFixture(t)
	ctx := context.Background()
	builtin := f.officeWorkspace(t, "ws-1")
	f.workflow(t, "ws-1", "wf-bare", nil)
	require.NoError(t, f.svc.repo.CreateStep(ctx, &models.WorkflowStep{
		ID: "bare-1", WorkflowID: "wf-bare", Name: "Only", Position: 0,
		Events: models.StepEvents{OnEnter: []models.OnEnterAction{
			{Type: models.OnEnterRunCodeReview, Config: map[string]interface{}{"agent_profile_id": "profile-missing"}},
		}},
	}))

	err := f.svc.SelectOfficeWorkflow(ctx, "ws-1", "wf-bare")
	assert.Equal(t, []string{
		models.OfficeProblemStartStepCount, models.OfficeProblemMissingWorkStage,
		models.OfficeProblemUnknownAgentProfile, models.OfficeProblemFinalStepNotCompleting,
	}, refusalCodes(t, err))
	assert.Equal(t, builtin, f.selected(t, "ws-1"))
	assert.False(t, f.isOffice(t, "wf-bare"))
}

// @covers AC-OFFICE-WORKFLOW-SELECT-004.4
func TestSelectOfficeWorkflowRefusesIneligibleWorkflows(t *testing.T) {
	f := newOfficeFixture(t)
	ctx := context.Background()
	builtin := f.officeWorkspace(t, "ws-1")
	f.officeWorkspace(t, "ws-2")
	f.workflow(t, "ws-1", "wf-kanban-busy", nil)
	f.validSteps(t, "wf-kanban-busy")
	require.NoError(t, f.tasks.CreateTask(ctx, &taskmodels.Task{
		ID: "t-kanban", WorkspaceID: "ws-1", WorkflowID: "wf-kanban-busy", Title: "k", State: "TODO",
	}))
	f.workflow(t, "ws-1", "wf-synced", func(w *taskmodels.Workflow) {
		w.Source = taskmodels.WorkflowSourceGitHub
		w.SourcePath = "office.yml"
	})
	f.validSteps(t, "wf-synced")
	f.workflow(t, "ws-2", "wf-foreign", nil)
	f.validSteps(t, "wf-foreign")

	for workflowID, code := range map[string]string{
		"wf-kanban-busy": models.OfficeProblemHoldsKanbanTasks,
		"wf-synced":      models.OfficeProblemSyncManaged,
		"wf-foreign":     models.OfficeProblemNotInWorkspace,
		"wf-missing":     models.OfficeProblemNotInWorkspace,
	} {
		t.Run(workflowID, func(t *testing.T) {
			err := f.svc.SelectOfficeWorkflow(ctx, "ws-1", workflowID)
			assert.Equal(t, []string{code}, refusalCodes(t, err))
			assert.Equal(t, builtin, f.selected(t, "ws-1"))
		})
	}
}

func TestSelectOfficeWorkflowRequiresOfficeWorkspace(t *testing.T) {
	f := newOfficeFixture(t)
	ctx := context.Background()
	require.NoError(t, f.tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-kanban", Name: "k", OwnerID: "u-1"}))
	f.workflow(t, "ws-kanban", "wf-1", nil)
	f.validSteps(t, "wf-1")

	err := f.svc.SelectOfficeWorkflow(ctx, "ws-kanban", "wf-1")
	assert.Equal(t, []string{models.OfficeProblemRequiresOfficeWorkspace}, refusalCodes(t, err))
	assert.Empty(t, f.selected(t, "ws-kanban"))
}

// @covers AC-OFFICE-WORKFLOW-SELECT-004.5
func TestSelectOfficeWorkflowRequiresWorkspaceManage(t *testing.T) {
	f := newOfficeFixture(t)
	ctx := context.Background()
	builtin := f.officeWorkspace(t, "ws-1")
	f.workflow(t, "ws-1", "wf-copy", nil)
	f.validSteps(t, "wf-copy")
	denied := errors.New("forbidden")
	f.svc.SetWorkspaceManageChecker(func(context.Context, string) error { return denied })

	err := f.svc.SelectOfficeWorkflow(ctx, "ws-1", "wf-copy")
	require.ErrorIs(t, err, denied)
	assert.Equal(t, builtin, f.selected(t, "ws-1"))
}

// @covers AC-OFFICE-WORKFLOW-SELECT-004.1
func TestOfficeWorkflowStateListsCandidates(t *testing.T) {
	f := newOfficeFixture(t)
	ctx := context.Background()
	builtin := f.officeWorkspace(t, "ws-1")
	f.workflow(t, "ws-1", "wf-copy", func(w *taskmodels.Workflow) { w.IsOffice = true; w.Name = "Copy" })
	f.workflow(t, "ws-1", "wf-empty", func(w *taskmodels.Workflow) { w.Name = "Empty" })
	f.workflow(t, "ws-1", "wf-busy", nil)
	require.NoError(t, f.tasks.CreateTask(ctx, &taskmodels.Task{
		ID: "t-busy", WorkspaceID: "ws-1", WorkflowID: "wf-busy", Title: "b", State: "TODO",
	}))
	f.workflow(t, "ws-1", "wf-synced", func(w *taskmodels.Workflow) { w.Source = taskmodels.WorkflowSourceGitHub })
	_, err := f.tasks.EnsureRoutineWorkflow(ctx, "ws-1")
	require.NoError(t, err)

	state, err := f.svc.OfficeWorkflowState(ctx, "ws-1")
	require.NoError(t, err)
	assert.Equal(t, builtin, state.SelectedWorkflowID)
	byID := map[string]OfficeWorkflowCandidate{}
	for _, c := range state.Candidates {
		byID[c.ID] = c
	}
	assert.Len(t, byID, 3, "candidates: %+v", state.Candidates)
	assert.True(t, byID[builtin].BuiltIn && byID[builtin].IsOffice)
	assert.True(t, byID["wf-copy"].IsOffice && !byID["wf-copy"].BuiltIn)
	assert.False(t, byID["wf-empty"].IsOffice)
}
