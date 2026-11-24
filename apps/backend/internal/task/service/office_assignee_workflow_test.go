package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-OFFICE-WORKFLOW-SELECT-001.1
func TestValidateAssigneeTaskIsOfficeAcceptsAnyOfficeWorkflow(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS", OfficeWorkflowID: "wf-selected"})
	for _, wf := range []*models.Workflow{
		{ID: "wf-selected", WorkspaceID: "ws-1", Name: "Selected", IsOffice: true},
		{ID: "wf-previous", WorkspaceID: "ws-1", Name: "Previous", IsOffice: true},
		{ID: "wf-kanban", WorkspaceID: "ws-1", Name: "Kanban"},
	} {
		if err := repo.CreateWorkflow(ctx, wf); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}

	for _, tc := range []struct {
		workflowID string
		wantErr    bool
	}{
		{"wf-selected", false},
		{"wf-previous", false},
		{"wf-kanban", true},
	} {
		t.Run(tc.workflowID, func(t *testing.T) {
			err := svc.validateAssigneeTaskIsOffice(ctx, &CreateTaskRequest{
				WorkspaceID: "ws-1", WorkflowID: tc.workflowID, AssigneeAgentProfileID: "agent-1",
			})
			if gotErr := errors.Is(err, ErrInvalidAssigneeAgentProfile); gotErr != tc.wantErr {
				t.Fatalf("err = %v, want invalid-assignee=%v", err, tc.wantErr)
			}
		})
	}
}

// @covers AC-OFFICE-WORKFLOW-SELECT-002.5
func TestDeleteWorkflowRefusesBuiltInAndSelectedOffice(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS", OfficeWorkflowID: "wf-selected"})
	for _, wf := range []*models.Workflow{
		{ID: "wf-selected", WorkspaceID: "ws-1", Name: "Selected", IsOffice: true},
		{ID: "wf-copy", WorkspaceID: "ws-1", Name: "Unselected copy", IsOffice: true},
	} {
		if err := repo.CreateWorkflow(ctx, wf); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}
	builtin, err := repo.EnsureOfficeDefaultWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("EnsureOfficeDefaultWorkflow: %v", err)
	}

	for id, code := range map[string]string{builtin: "built_in_read_only", "wf-selected": "selected_delete"} {
		err := svc.DeleteWorkflow(ctx, id)
		var refusal *OfficeWorkflowDeleteRefusal
		if !errors.As(err, &refusal) || refusal.Code != code {
			t.Fatalf("DeleteWorkflow(%s) err = %v, want refusal %s", id, err, code)
		}
		if _, err := repo.GetWorkflow(ctx, id); err != nil {
			t.Fatalf("workflow %s was deleted: %v", id, err)
		}
	}
	if err := svc.DeleteWorkflow(ctx, "wf-copy"); err != nil {
		t.Fatalf("DeleteWorkflow unselected copy: %v", err)
	}
}

// @covers AC-OFFICE-WORKFLOW-SELECT-006.4, AC-OFFICE-WORKFLOW-SELECT-003.4
func TestOfficeStepWriteRefusal(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	_ = repo.CreateWorkspace(ctx, &models.Workspace{ID: "ws-1", Name: "WS", OfficeWorkflowID: "wf-selected"})
	for _, wf := range []*models.Workflow{
		{ID: "wf-selected", WorkspaceID: "ws-1", Name: "Selected", IsOffice: true},
		{ID: "wf-copy", WorkspaceID: "ws-1", Name: "Copy", IsOffice: true},
		{ID: "wf-kanban", WorkspaceID: "ws-1", Name: "Kanban"},
	} {
		if err := repo.CreateWorkflow(ctx, wf); err != nil {
			t.Fatalf("CreateWorkflow: %v", err)
		}
	}
	builtin, err := repo.EnsureOfficeDefaultWorkflow(ctx, "ws-1")
	if err != nil {
		t.Fatalf("EnsureOfficeDefaultWorkflow: %v", err)
	}
	for id, want := range map[string]string{
		builtin: "built_in_read_only", "wf-selected": "selected_requires_definition_save", "wf-copy": "", "wf-kanban": "",
	} {
		got, err := svc.OfficeStepWriteRefusal(ctx, id)
		if err != nil || got != want {
			t.Errorf("OfficeStepWriteRefusal(%s) = %q, %v; want %q", id, got, err, want)
		}
	}
}
