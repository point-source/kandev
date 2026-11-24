package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	taskmodels "github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/service"
)

// Definition saves answer the same permission statuses as other workflow
// writes instead of a 500.
func TestOfficeDefinitionSavePermissionStatuses(t *testing.T) {
	f := newOfficeHandlerFixture(t)
	rec := f.do(t, http.MethodPost, "/api/v1/workspaces/ws-1/office-workflows", officeDraftBody("Copy", true))
	require.Equal(t, http.StatusCreated, rec.Code, rec.Body.String())
	var created service.OfficeWorkflowSaveResult
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &created))
	path := "/api/v1/workflows/" + created.WorkflowID + "/definition"
	body := officeDraftBody("Copy", true)

	// Cases run in order; each resets the checker the previous one set.
	tests := []struct {
		name      string
		configure func()
		status    int
	}{
		{
			name: "workspace manager denied",
			configure: func() {
				f.svc.SetWorkflowMutationReadOnlyChecker(func(context.Context, string) (bool, bool, error) {
					return false, false, fmt.Errorf("%w: %w", service.ErrWorkspaceManageDenied, errors.New("forbidden"))
				})
			},
			status: http.StatusForbidden,
		},
		{
			name: "executor denied",
			configure: func() {
				f.svc.SetWorkflowMutationReadOnlyChecker(nil)
				f.svc.SetExecutorMutationChecker(func(context.Context, string) error {
					return taskmodels.ErrExecutorMutationDenied
				})
			},
			status: http.StatusForbidden,
		},
		{
			name: "workspace read-only",
			configure: func() {
				f.svc.SetExecutorMutationChecker(nil)
				f.svc.SetWorkflowMutationReadOnlyChecker(func(context.Context, string) (bool, bool, error) {
					return false, true, nil
				})
			},
			status: http.StatusConflict,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.configure()
			rec := f.do(t, http.MethodPut, path, body)
			assert.Equal(t, tc.status, rec.Code, rec.Body.String())
		})
	}
}
