package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
)

func moveToStepEvents(target string) models.StepEvents {
	return models.StepEvents{OnTurnComplete: []models.OnTurnCompleteAction{
		{Type: models.OnTurnCompleteMoveToStep, Config: map[string]interface{}{"step_id": target}},
	}}
}

// draftSteps is a valid Office definition whose steps use client keys.
func draftSteps() []*models.WorkflowStep {
	return []*models.WorkflowStep{
		{
			ID: "temp-step-work", Name: "Work", Position: 0, Color: "bg-blue-500", IsStartStep: true,
			StageType: models.StageTypeWork, AutoAdvanceRequiresSignal: true, AllowManualMove: true,
			Events: moveToStepEvents("temp-step-done"),
		},
		{
			ID: "temp-step-done", Name: "Done", Position: 1, Color: "bg-green-500",
			StageType: models.StageTypeCustom, CompleteTaskOnEnter: true,
		},
	}
}

func (f *officeFixture) createCopy(t *testing.T, workspaceID string) *OfficeWorkflowSaveResult {
	t.Helper()
	result, err := f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
		WorkspaceID: workspaceID, Name: "Office copy", Description: "copied", Steps: draftSteps(),
	})
	require.NoError(t, err)
	return result
}

func stepNamed(steps []*models.WorkflowStep, name string) *models.WorkflowStep {
	for _, s := range steps {
		if s.Name == name {
			return s
		}
	}
	return nil
}

// @covers AC-OFFICE-WORKFLOW-SELECT-002.2
func TestSaveOfficeDefinitionCreatesOfficeCopyWithRemappedReferences(t *testing.T) {
	f := newOfficeFixture(t)
	f.officeWorkspace(t, "ws-1")
	result := f.createCopy(t, "ws-1")

	wf, err := f.tasks.GetWorkflow(context.Background(), result.WorkflowID)
	require.NoError(t, err)
	assert.True(t, wf.IsOffice)
	assert.False(t, wf.IsSystem)
	assert.Equal(t, taskmodels.WorkflowStyleOffice, wf.Style)
	assert.Equal(t, "Office copy", wf.Name)
	assert.Nil(t, wf.WorkflowTemplateID)

	steps, err := f.svc.repo.ListStepsByWorkflow(context.Background(), result.WorkflowID)
	require.NoError(t, err)
	require.Len(t, steps, 2)
	work, done := stepNamed(steps, "Work"), stepNamed(steps, "Done")
	assert.NotEqual(t, "temp-step-done", done.ID)
	assert.Equal(t, done.ID, work.Events.OnTurnComplete[0].Config["step_id"])
	assert.True(t, work.IsStartStep && work.AutoAdvanceRequiresSignal)
	assert.Equal(t, models.StageTypeWork, work.StageType)
}

func TestSaveOfficeDefinitionRequiresOfficeWorkspace(t *testing.T) {
	f := newOfficeFixture(t)
	require.NoError(t, f.tasks.CreateWorkspace(context.Background(), &taskmodels.Workspace{ID: "ws-k", Name: "k", OwnerID: "u"}))
	_, err := f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
		WorkspaceID: "ws-k", Name: "x", Steps: draftSteps(),
	})
	assert.Equal(t, []string{models.OfficeProblemRequiresOfficeWorkspace}, refusalCodes(t, err))
}

// An unselected copy may be saved in a shape Office cannot drive yet.
func TestSaveOfficeDefinitionAllowsInvalidUnselectedCopy(t *testing.T) {
	f := newOfficeFixture(t)
	f.officeWorkspace(t, "ws-1")
	created := f.createCopy(t, "ws-1")
	steps := created.Steps
	steps[1].CompleteTaskOnEnter = false
	_, err := f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
		WorkflowID: created.WorkflowID, Name: "Office copy", Steps: steps,
	})
	require.NoError(t, err)
}

// @covers AC-OFFICE-WORKFLOW-SELECT-006.1
func TestSelectedSaveRefusedWhenInvalid(t *testing.T) {
	f := newOfficeFixture(t)
	ctx := context.Background()
	f.officeWorkspace(t, "ws-1")
	created := f.createCopy(t, "ws-1")
	require.NoError(t, f.svc.SelectOfficeWorkflow(ctx, "ws-1", created.WorkflowID))

	broken := created.Steps
	broken[1].CompleteTaskOnEnter = false
	broken[1].Name = "Renamed"
	_, err := f.svc.SaveOfficeWorkflowDefinition(ctx, OfficeWorkflowDefinition{
		WorkflowID: created.WorkflowID, Name: "Office copy", Steps: broken,
	})
	assert.Equal(t, []string{models.OfficeProblemFinalStepNotCompleting}, refusalCodes(t, err))

	stored, err := f.svc.repo.ListStepsByWorkflow(ctx, created.WorkflowID)
	require.NoError(t, err)
	assert.NotNil(t, stepNamed(stored, "Done"), "nothing was written")
	assert.True(t, stepNamed(stored, "Done").CompleteTaskOnEnter)
}

// @covers AC-OFFICE-WORKFLOW-SELECT-006.2
func TestSaveOfficeDefinitionAppliesAddRenameRemoveTogether(t *testing.T) {
	f := newOfficeFixture(t)
	ctx := context.Background()
	f.officeWorkspace(t, "ws-1")
	created := f.createCopy(t, "ws-1")
	require.NoError(t, f.svc.SelectOfficeWorkflow(ctx, "ws-1", created.WorkflowID))
	work, done := stepNamed(created.Steps, "Work"), stepNamed(created.Steps, "Done")

	work.Events = moveToStepEvents("temp-step-verify")
	verify := &models.WorkflowStep{
		ID: "temp-step-verify", Name: "Verify", Position: 1, StageType: models.StageTypeReview,
		Events: models.StepEvents{
			OnEnter: []models.OnEnterAction{
				{Type: models.OnEnterEnsureParticipantSeat, Config: map[string]interface{}{"role": "reviewer"}},
				{Type: models.OnEnterQueueRunForEachParticipant, Config: map[string]interface{}{"role": "reviewer"}},
				{Type: models.OnEnterRunCodeReview, Config: map[string]interface{}{"agent_profile_id": "profile-ok"}},
			},
			OnTurnComplete: moveToStepEvents(done.ID).OnTurnComplete,
		},
	}
	done.Position = 2
	done.Name = "Shipped"
	result, err := f.svc.SaveOfficeWorkflowDefinition(ctx, OfficeWorkflowDefinition{
		WorkflowID: created.WorkflowID, Name: "Office copy v2", Steps: []*models.WorkflowStep{work, verify, done},
	})
	require.NoError(t, err)
	assert.Len(t, result.Created, 1)
	assert.Len(t, result.Updated, 2)

	stored, err := f.svc.repo.ListStepsByWorkflow(ctx, created.WorkflowID)
	require.NoError(t, err)
	require.Len(t, stored, 3)
	assert.Equal(t, done.ID, stepNamed(stored, "Shipped").ID, "kept steps keep their identity")
	storedVerify := stepNamed(stored, "Verify")
	assert.Equal(t, storedVerify.ID, stepNamed(stored, "Work").Events.OnTurnComplete[0].Config["step_id"])
	wf, err := f.tasks.GetWorkflow(ctx, created.WorkflowID)
	require.NoError(t, err)
	assert.Equal(t, "Office copy v2", wf.Name)

	// Removing the Verify step again (no tasks on it) is allowed.
	work.Events = moveToStepEvents(done.ID)
	done.Position = 1
	_, err = f.svc.SaveOfficeWorkflowDefinition(ctx, OfficeWorkflowDefinition{
		WorkflowID: created.WorkflowID, Name: "Office copy v2", Steps: []*models.WorkflowStep{work, done},
	})
	require.NoError(t, err)
	stored, err = f.svc.repo.ListStepsByWorkflow(ctx, created.WorkflowID)
	require.NoError(t, err)
	assert.Len(t, stored, 2)
}

// @covers AC-OFFICE-WORKFLOW-SELECT-006.3
func TestSaveRefusesRemovingStepWithTasks(t *testing.T) {
	f := newOfficeFixture(t)
	ctx := context.Background()
	f.officeWorkspace(t, "ws-1")
	created := f.createCopy(t, "ws-1")
	work, done := stepNamed(created.Steps, "Work"), stepNamed(created.Steps, "Done")
	require.NoError(t, f.tasks.CreateTask(ctx, &taskmodels.Task{
		ID: "t-1", WorkspaceID: "ws-1", WorkflowID: created.WorkflowID, WorkflowStepID: done.ID, Title: "t", State: "TODO",
	}))
	work.Events = models.StepEvents{}
	_, err := f.svc.SaveOfficeWorkflowDefinition(ctx, OfficeWorkflowDefinition{
		WorkflowID: created.WorkflowID, Name: "Office copy", Steps: []*models.WorkflowStep{work},
	})
	refusal, ok := AsOfficeWorkflowRefusal(err)
	require.True(t, ok, "err = %v", err)
	assert.Equal(t, []models.OfficeWorkflowProblem{{Code: models.OfficeProblemStepHoldsTasks, StepName: "Done"}}, refusal.Problems)
}

// @covers AC-OFFICE-WORKFLOW-SELECT-003.3
func TestDefinitionSaveRejectsInvalidEvents(t *testing.T) {
	f := newOfficeFixture(t)
	f.officeWorkspace(t, "ws-1")
	cases := map[string]func([]*models.WorkflowStep){
		"unknown action": func(s []*models.WorkflowStep) {
			s[1].Events.OnEnter = []models.OnEnterAction{{Type: "ensure_seat"}}
		},
		"reference to a missing step": func(s []*models.WorkflowStep) {
			s[0].Events = moveToStepEvents("temp-step-gone")
		},
		"duplicate step names": func(s []*models.WorkflowStep) { s[1].Name = "Work" },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			steps := draftSteps()
			mutate(steps)
			_, err := f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
				WorkspaceID: "ws-1", Name: "bad " + name, Steps: steps,
			})
			codes := refusalCodes(t, err)
			assert.Equal(t, []string{models.OfficeProblemInvalidEvents}, codes)
		})
	}
	state, err := f.svc.OfficeWorkflowState(context.Background(), "ws-1")
	require.NoError(t, err)
	assert.Len(t, state.Candidates, 1, "no workflow was created")
}

// @covers AC-OFFICE-WORKFLOW-SELECT-003.4
func TestSaveOfficeDefinitionRefusesBuiltInAndKanbanTargets(t *testing.T) {
	f := newOfficeFixture(t)
	builtin := f.officeWorkspace(t, "ws-1")
	f.workflow(t, "ws-1", "wf-kanban", nil)
	for id, code := range map[string]string{
		builtin:     models.OfficeProblemBuiltInReadOnly,
		"wf-kanban": models.OfficeProblemNotOfficeWorkflow,
	} {
		_, err := f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
			WorkflowID: id, Name: "x", Steps: draftSteps(),
		})
		refusal, ok := AsOfficeWorkflowRefusal(err)
		require.True(t, ok, "err = %v", err)
		assert.True(t, refusal.Conflict)
		assert.Equal(t, code, refusal.Problems[0].Code)
	}
}

func TestSaveOfficeDefinitionDetectsConcurrentEdit(t *testing.T) {
	f := newOfficeFixture(t)
	f.officeWorkspace(t, "ws-1")
	created := f.createCopy(t, "ws-1")
	stale := created.UpdatedAt.Add(-time.Minute)
	_, err := f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
		WorkflowID: created.WorkflowID, ExpectedUpdatedAt: &stale, Name: "x", Steps: created.Steps,
	})
	assert.True(t, errors.Is(err, repoerrors.ErrTaskVersionConflict), "err = %v", err)
}
