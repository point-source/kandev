package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
)

func copySteps(steps []*models.WorkflowStep) []*models.WorkflowStep {
	out := make([]*models.WorkflowStep, 0, len(steps))
	for _, step := range steps {
		copied := *step
		out = append(out, &copied)
	}
	return out
}

func stepVersions(steps []*models.WorkflowStep) map[string]time.Time {
	versions := make(map[string]time.Time, len(steps))
	for _, step := range steps {
		versions[step.ID] = step.UpdatedAt
	}
	return versions
}

// Validation and the write must see the same step set: a repeated step ID
// would validate one entry and persist the other.
func TestSaveOfficeDefinitionRejectsDuplicateStepIDs(t *testing.T) {
	f := newOfficeFixture(t)
	f.officeWorkspace(t, "ws-1")
	created := f.createCopy(t, "ws-1")
	steps := copySteps(created.Steps)
	shadow := *stepNamed(steps, "Work")
	shadow.Name, shadow.IsStartStep, shadow.StageType = "Shadow", false, models.StageTypeCustom
	steps = append(steps, &shadow)

	_, err := f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
		WorkflowID: created.WorkflowID, Name: "Office copy", Steps: steps,
	})
	assert.Equal(t, []string{models.OfficeProblemInvalidEvents}, refusalCodes(t, err))
	stored, err := f.svc.repo.ListStepsByWorkflow(context.Background(), created.WorkflowID)
	require.NoError(t, err)
	work := stepNamed(stored, "Work")
	require.NotNil(t, work)
	assert.True(t, work.IsStartStep)
}

func TestSaveOfficeDefinitionTakesPositionsFromOrder(t *testing.T) {
	f := newOfficeFixture(t)
	f.officeWorkspace(t, "ws-1")
	steps := draftSteps()
	for _, step := range steps {
		step.Position = 0
	}
	result, err := f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
		WorkspaceID: "ws-1", Name: "Office copy", Steps: steps,
	})
	require.NoError(t, err)
	stored, err := f.svc.repo.ListStepsByWorkflow(context.Background(), result.WorkflowID)
	require.NoError(t, err)
	assert.Equal(t, 0, stepNamed(stored, "Work").Position)
	assert.Equal(t, 1, stepNamed(stored, "Done").Position)
}

func TestSaveOfficeDefinitionAppliesRoutingTopologyRules(t *testing.T) {
	f := newOfficeFixture(t)
	f.officeWorkspace(t, "ws-1")
	tests := map[string]func([]*models.WorkflowStep){
		"self pull": func(steps []*models.WorkflowStep) {
			steps[1].PullFromStepID = steps[1].ID
		},
		"pull cycle": func(steps []*models.WorkflowStep) {
			steps[0].PullFromStepID = steps[1].ID
			steps[1].PullFromStepID = steps[0].ID
		},
		"session source after target": func(steps []*models.WorkflowStep) {
			steps[1].AgentProfileID = "profile-ok"
			steps[0].SessionTarget = &models.WorkflowSessionTarget{Kind: models.WorkflowSessionTargetStep, StepID: steps[1].ID}
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			steps := draftSteps()
			mutate(steps)
			_, err := f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
				WorkspaceID: "ws-1", Name: "Office copy " + name, Steps: steps,
			})
			assert.Equal(t, []string{models.OfficeProblemInvalidEvents}, refusalCodes(t, err))
		})
	}
}

// An editor opened before another writer changed a step must not overwrite
// that change, and a returned version must be usable as the next expected
// version.
func TestSaveOfficeDefinitionRefusesChangedStepVersions(t *testing.T) {
	f := newOfficeFixture(t)
	f.officeWorkspace(t, "ws-1")
	created := f.createCopy(t, "ws-1")
	save := func(updatedAt time.Time, versions map[string]time.Time, name string) (*OfficeWorkflowSaveResult, error) {
		return f.svc.SaveOfficeWorkflowDefinition(context.Background(), OfficeWorkflowDefinition{
			WorkflowID: created.WorkflowID, Name: name, Steps: copySteps(created.Steps),
			ExpectedUpdatedAt: &updatedAt, ExpectedSteps: versions,
		})
	}
	fresh, err := save(created.UpdatedAt, stepVersions(created.Steps), "Renamed once")
	require.NoError(t, err)

	done := stepNamed(fresh.Steps, "Done")
	_, err = f.svc.repo.UpdateStepEventsIfUnchanged(context.Background(), []repository.StepEventsUpdate{{
		StepID: done.ID, WorkflowID: created.WorkflowID, ExpectedUpdatedAt: done.UpdatedAt,
	}})
	require.NoError(t, err)

	_, err = save(fresh.UpdatedAt, stepVersions(fresh.Steps), "Stale rename")
	assert.True(t, errors.Is(err, repoerrors.ErrTaskVersionConflict), "err = %v", err)
}

// A single-step write whose early check passed before the workflow became
// selected is refused by the check inside its transaction.
func TestGuardedStepWriteRechecksSelectionInTransaction(t *testing.T) {
	f := newOfficeFixture(t)
	f.officeWorkspace(t, "ws-1")
	created := f.createCopy(t, "ws-1")
	require.NoError(t, f.svc.SelectOfficeWorkflow(context.Background(), "ws-1", created.WorkflowID))
	// A policy that answers from a stale read, as one racing the selection would.
	f.svc.SetOfficeStepWritePolicy(func(context.Context, string) (string, error) { return "", nil })

	step, err := f.svc.GetStep(context.Background(), stepNamed(created.Steps, "Work").ID)
	require.NoError(t, err)
	step.Name = "Renamed"
	err = f.svc.UpdateStep(context.Background(), step)
	assert.Equal(t, []string{models.OfficeProblemSelectedRequiresDefinitionSave}, refusalCodes(t, err))
	err = f.svc.DeleteStep(context.Background(), stepNamed(created.Steps, "Done").ID)
	assert.Equal(t, []string{models.OfficeProblemSelectedRequiresDefinitionSave}, refusalCodes(t, err))

	stored, err := f.svc.repo.ListStepsByWorkflow(context.Background(), created.WorkflowID)
	require.NoError(t, err)
	assert.Len(t, stored, 2)
	assert.NotNil(t, stepNamed(stored, "Work"))
}
