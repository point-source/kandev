package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/models"
)

type officeImportProvider struct{ f *officeFixture }

func (p officeImportProvider) ListWorkflows(ctx context.Context, workspaceID string, includeHidden bool) ([]*taskmodels.Workflow, error) {
	return p.f.tasks.ListWorkflows(ctx, workspaceID, includeHidden)
}

func (p officeImportProvider) GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error) {
	return p.f.tasks.GetWorkflow(ctx, id)
}

func (p officeImportProvider) CreateWorkflow(ctx context.Context, workspaceID, name, description string) (*taskmodels.Workflow, error) {
	wf := &taskmodels.Workflow{WorkspaceID: workspaceID, Name: name, Description: description}
	return wf, p.f.tasks.CreateWorkflow(ctx, wf)
}

func (p officeImportProvider) UpdateWorkflow(ctx context.Context, wf *taskmodels.Workflow) error {
	return p.f.tasks.UpdateWorkflow(ctx, wf)
}

func exportedOfficeCopy(t *testing.T, f *officeFixture, workspaceID string) *models.WorkflowExport {
	t.Helper()
	created := f.createCopy(t, workspaceID)
	export, err := f.svc.ExportWorkflows(context.Background(), workspaceID, []string{created.WorkflowID})
	require.NoError(t, err)
	require.Len(t, export.Workflows, 1)
	require.True(t, export.Workflows[0].Office)
	return export
}

// @covers AC-OFFICE-WORKFLOW-SELECT-007.2, AC-OFFICE-WORKFLOW-SELECT-007.4
func TestImportOfficeWorkflowCreatesSelectableOfficeCopy(t *testing.T) {
	f := newOfficeFixture(t)
	f.svc.SetWorkflowProvider(officeImportProvider{f: f})
	ctx := context.Background()
	f.officeWorkspace(t, "ws-src")
	f.officeWorkspace(t, "ws-dst")
	export := exportedOfficeCopy(t, f, "ws-src")

	for name, run := range map[string]func() (*ImportResult, error){
		"legacy": func() (*ImportResult, error) { return f.svc.ImportWorkflows(ctx, "ws-dst", export) },
		"bindings": func() (*ImportResult, error) {
			export.Workflows[0].Name = "Office copy (bindings)"
			return f.svc.ImportWorkflowsWithBindings(ctx, "ws-dst", export, nil)
		},
	} {
		t.Run(name, func(t *testing.T) {
			result, err := run()
			require.NoError(t, err)
			require.Len(t, result.Created, 1)
			imported := findWorkflowByName(t, f, "ws-dst", result.Created[0])
			assert.True(t, imported.IsOffice)
			steps, err := f.svc.repo.ListStepsByWorkflow(ctx, imported.ID)
			require.NoError(t, err)
			require.Len(t, steps, 2)
			assert.Equal(t, models.StageTypeWork, stepNamed(steps, "Work").StageType)
			assert.Equal(t, stepNamed(steps, "Done").ID, stepNamed(steps, "Work").Events.OnTurnComplete[0].Config["step_id"])
			require.NoError(t, f.svc.SelectOfficeWorkflow(ctx, "ws-dst", imported.ID))
		})
	}
	listed, err := f.tasks.ListWorkflows(ctx, "ws-dst", false)
	require.NoError(t, err)
	ids, err := f.tasks.ListOfficeWorkflowIDs(ctx)
	require.NoError(t, err)
	for _, wf := range listed {
		if wf.Name != "" && wf.IsOffice {
			_, excluded := ids[wf.ID]
			assert.True(t, excluded, "imported Office copy %q must stay off Kanban listings", wf.Name)
		}
	}
}

// @covers AC-OFFICE-WORKFLOW-SELECT-007.3
func TestImportOfficeWorkflowIntoKanbanWorkspaceIsRefused(t *testing.T) {
	f := newOfficeFixture(t)
	f.svc.SetWorkflowProvider(officeImportProvider{f: f})
	ctx := context.Background()
	f.officeWorkspace(t, "ws-src")
	require.NoError(t, f.tasks.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-kanban", Name: "k", OwnerID: "u"}))
	export := exportedOfficeCopy(t, f, "ws-src")
	export.Workflows = append(export.Workflows, models.WorkflowPortable{Name: "Plain", Steps: []models.StepPortable{
		{Name: "Todo", Position: 0, IsStartStep: true},
	}})

	_, err := f.svc.ImportWorkflowsWithBindings(ctx, "ws-kanban", export, nil)
	assert.Equal(t, []string{models.OfficeProblemRequiresOfficeWorkspace}, refusalCodes(t, err))
	_, err = f.svc.PreviewImportWorkflows(ctx, "ws-kanban", export)
	assert.Equal(t, []string{models.OfficeProblemRequiresOfficeWorkspace}, refusalCodes(t, err))
	listed, err := f.tasks.ListWorkflows(ctx, "ws-kanban", true)
	require.NoError(t, err)
	assert.Empty(t, listed, "nothing is written")
}

func findWorkflowByName(t *testing.T, f *officeFixture, workspaceID, name string) *taskmodels.Workflow {
	t.Helper()
	listed, err := f.tasks.ListWorkflows(context.Background(), workspaceID, true)
	require.NoError(t, err)
	for _, wf := range listed {
		if wf.Name == name {
			return wf
		}
	}
	t.Fatalf("workflow %q not found", name)
	return nil
}
