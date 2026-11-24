package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/jmoiron/sqlx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/db"
	"github.com/kandev/kandev/internal/events/bus"
	taskmodels "github.com/kandev/kandev/internal/task/models"
	tasksqlite "github.com/kandev/kandev/internal/task/repository/sqlite"
	"github.com/kandev/kandev/internal/workflow/controller"
	"github.com/kandev/kandev/internal/workflow/models"
	"github.com/kandev/kandev/internal/workflow/repository"
	"github.com/kandev/kandev/internal/workflow/service"
)

type officeHandlerFixture struct {
	router  *gin.Engine
	svc     *service.Service
	tasks   *tasksqlite.Repository
	builtin string
	changes []string
}

func newOfficeHandlerFixture(t *testing.T) *officeHandlerFixture {
	t.Helper()
	gin.SetMode(gin.TestMode)
	conn, err := db.OpenSQLite(filepath.Join(t.TempDir(), "office-handlers.db"))
	require.NoError(t, err)
	sqlxDB := sqlx.NewDb(conn, "sqlite3")
	t.Cleanup(func() { _ = sqlxDB.Close() })
	taskRepo, err := tasksqlite.NewWithDB(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	wfRepo, err := repository.NewWithDB(sqlxDB, sqlxDB, nil)
	require.NoError(t, err)
	log, _ := logger.NewLogger(logger.LoggingConfig{Level: "error", Format: "console"})
	svc := service.NewService(wfRepo, log)
	t.Cleanup(func() { _ = svc.Close() })
	f := &officeHandlerFixture{svc: svc, tasks: taskRepo}
	svc.SetWorkflowProvider(officeHandlerWorkflowProvider{tasks: taskRepo})
	svc.SetOfficeWorkflowChangePublisher(func(_ context.Context, workspaceID, workflowID string) {
		f.changes = append(f.changes, workspaceID+"/"+workflowID)
	})
	ctx := context.Background()
	require.NoError(t, taskRepo.CreateWorkspace(ctx, &taskmodels.Workspace{ID: "ws-1", Name: "ws", OwnerID: "u-1"}))
	f.builtin, err = taskRepo.EnsureOfficeWorkflow(ctx, "ws-1")
	require.NoError(t, err)
	f.router = gin.New()
	NewHandlers(controller.NewController(svc), bus.NewMemoryEventBus(log), log).registerHTTP(f.router)
	return f
}

func (f *officeHandlerFixture) do(t *testing.T, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var reader *bytes.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		require.NoError(t, err)
		reader = bytes.NewReader(raw)
	} else {
		reader = bytes.NewReader(nil)
	}
	req := httptest.NewRequest(method, path, reader)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	return rec
}

func TestOfficeWorkflowHTTPGetListsSelectionAndCandidates(t *testing.T) {
	f := newOfficeHandlerFixture(t)
	rec := f.do(t, http.MethodGet, "/api/v1/workspaces/ws-1/office-workflow", nil)
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	var state service.OfficeWorkflowState
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &state))
	assert.Equal(t, f.builtin, state.SelectedWorkflowID)
	require.Len(t, state.Candidates, 1)
	assert.True(t, state.Candidates[0].BuiltIn)
}

// @covers AC-OFFICE-WORKFLOW-SELECT-004.4, AC-OFFICE-WORKFLOW-SELECT-005.6
func TestOfficeWorkflowHTTPPutRefusalReturnsProblems(t *testing.T) {
	f := newOfficeHandlerFixture(t)
	require.NoError(t, f.tasks.CreateWorkflow(context.Background(), &taskmodels.Workflow{ID: "wf-empty", WorkspaceID: "ws-1", Name: "Empty"}))

	rec := f.do(t, http.MethodPut, "/api/v1/workspaces/ws-1/office-workflow", map[string]string{"workflow_id": "wf-empty"})
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	var body struct {
		Code     string                         `json:"code"`
		Problems []models.OfficeWorkflowProblem `json:"problems"`
	}
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.Equal(t, "office_workflow_refused", body.Code)
	require.NotEmpty(t, body.Problems)
	assert.Equal(t, models.OfficeProblemStartStepCount, body.Problems[0].Code)
	assert.Empty(t, f.changes)
}

// @covers AC-OFFICE-WORKFLOW-SELECT-004.2
func TestOfficeWorkflowHTTPPutSelectsAndPublishes(t *testing.T) {
	f := newOfficeHandlerFixture(t)
	rec := f.do(t, http.MethodPut, "/api/v1/workspaces/ws-1/office-workflow", map[string]string{"workflow_id": f.builtin})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())
	assert.Equal(t, []string{"ws-1/" + f.builtin}, f.changes)
}

// @covers AC-OFFICE-WORKFLOW-SELECT-004.5
func TestOfficeWorkflowHTTPPutForbidden(t *testing.T) {
	f := newOfficeHandlerFixture(t)
	f.svc.SetWorkspaceManageChecker(func(context.Context, string) error { return service.ErrWorkspaceManageDenied })
	rec := f.do(t, http.MethodPut, "/api/v1/workspaces/ws-1/office-workflow", map[string]string{"workflow_id": f.builtin})
	assert.Equal(t, http.StatusForbidden, rec.Code, rec.Body.String())
}

func TestOfficeWorkflowHTTPPutRequiresWorkflowID(t *testing.T) {
	f := newOfficeHandlerFixture(t)
	rec := f.do(t, http.MethodPut, "/api/v1/workspaces/ws-1/office-workflow", map[string]string{})
	assert.Equal(t, http.StatusBadRequest, rec.Code, rec.Body.String())
}

func officeDraftBody(name string, completing bool) map[string]any {
	return map[string]any{
		"name": name,
		"steps": []map[string]any{
			{
				"id": "temp-step-work", "name": "Work", "position": 0, "is_start_step": true,
				"stage_type": "work", "auto_advance_requires_signal": true,
				"events": map[string]any{"on_turn_complete": []map[string]any{
					{"type": "move_to_step", "config": map[string]any{"step_id": "temp-step-done"}},
				}},
			},
			{"id": "temp-step-done", "name": "Done", "position": 1, "complete_task_on_enter": completing},
		},
	}
}

// @covers AC-OFFICE-WORKFLOW-SELECT-002.2, AC-OFFICE-WORKFLOW-SELECT-006.1
func TestOfficeWorkflowHTTPDefinitionCreateAndGuardedSave(t *testing.T) {
	f := newOfficeHandlerFixture(t)
	rec := f.do(t, http.MethodPost, "/api/v1/workspaces/ws-1/office-workflows", officeDraftBody("Copy", true))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created service.OfficeWorkflowSaveResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	require.Len(t, created.Steps, 2)

	rec = f.do(t, http.MethodPut, "/api/v1/workspaces/ws-1/office-workflow", map[string]string{"workflow_id": created.WorkflowID})
	require.Equal(t, http.StatusOK, rec.Code, rec.Body.String())

	// Single-step writes to the selected workflow are refused with a code.
	f.svc.SetOfficeStepWritePolicy(func(_ context.Context, id string) (string, error) {
		if id == created.WorkflowID {
			return models.OfficeProblemSelectedRequiresDefinitionSave, nil
		}
		return "", nil
	})
	rec = f.do(t, http.MethodDelete, "/api/v1/workflow/steps/"+created.Steps[1].ID, nil)
	require.Equal(t, http.StatusConflict, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), models.OfficeProblemSelectedRequiresDefinitionSave)

	body := officeDraftBody("Copy", false)
	steps := body["steps"].([]map[string]any)
	steps[0]["id"], steps[1]["id"] = created.Steps[0].ID, created.Steps[1].ID
	steps[0]["events"] = map[string]any{"on_turn_complete": []map[string]any{
		{"type": "move_to_step", "config": map[string]any{"step_id": created.Steps[1].ID}},
	}}
	rec = f.do(t, http.MethodPut, "/api/v1/workflows/"+created.WorkflowID+"/definition", body)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), models.OfficeProblemFinalStepNotCompleting)

	rec = f.do(t, http.MethodPut, "/api/v1/workflows/missing/definition", officeDraftBody("x", true))
	assert.Equal(t, http.StatusNotFound, rec.Code, rec.Body.String())
}

// @covers AC-OFFICE-WORKFLOW-SELECT-007.3
func TestOfficeWorkflowHTTPImportIntoKanbanWorkspaceRefused(t *testing.T) {
	f := newOfficeHandlerFixture(t)
	require.NoError(t, f.tasks.CreateWorkspace(context.Background(), &taskmodels.Workspace{ID: "ws-k", Name: "k", OwnerID: "u"}))
	yamlBody := "version: 1\ntype: kandev_workflow\nworkflows:\n  - name: Office copy\n    office: true\n    steps:\n      - name: Work\n        position: 0\n        is_start_step: true\n"
	req := httptest.NewRequest(http.MethodPost, "/api/v1/workspaces/ws-k/workflows/import", bytes.NewReader([]byte(yamlBody)))
	req.Header.Set("Content-Type", "application/x-yaml")
	rec := httptest.NewRecorder()
	f.router.ServeHTTP(rec, req)
	require.Equal(t, http.StatusUnprocessableEntity, rec.Code, rec.Body.String())
	assert.Contains(t, rec.Body.String(), models.OfficeProblemRequiresOfficeWorkspace)
}

type officeHandlerWorkflowProvider struct{ tasks *tasksqlite.Repository }

func (p officeHandlerWorkflowProvider) ListWorkflows(ctx context.Context, workspaceID string, includeHidden bool) ([]*taskmodels.Workflow, error) {
	return p.tasks.ListWorkflows(ctx, workspaceID, includeHidden)
}

func (p officeHandlerWorkflowProvider) GetWorkflow(ctx context.Context, id string) (*taskmodels.Workflow, error) {
	return p.tasks.GetWorkflow(ctx, id)
}

func (p officeHandlerWorkflowProvider) CreateWorkflow(ctx context.Context, workspaceID, name, description string) (*taskmodels.Workflow, error) {
	wf := &taskmodels.Workflow{WorkspaceID: workspaceID, Name: name, Description: description}
	return wf, p.tasks.CreateWorkflow(ctx, wf)
}

func (p officeHandlerWorkflowProvider) UpdateWorkflow(ctx context.Context, wf *taskmodels.Workflow) error {
	return p.tasks.UpdateWorkflow(ctx, wf)
}
