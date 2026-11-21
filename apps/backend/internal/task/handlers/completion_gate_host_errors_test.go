package handlers

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository"
	"github.com/kandev/kandev/internal/task/service"
	"github.com/stretchr/testify/require"
)

// @covers AC-TASKS-COMPLETION-006.2, AC-TASKS-COMPLETION-006.8
func TestHTTPCompletionGateHostErrorsHaveStableCodes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	repo := &completionGateResponseRepository{
		moveTaskConflictRepo: &moveTaskConflictRepo{task: &models.Task{ID: "task-1"}},
		snapshot:             &models.TaskCompletionGateSnapshot{TaskID: "task-1"},
	}
	log := newTestLogger(t)
	handlers := &TaskHandlers{service: service.NewService(service.Repos{Tasks: repo}, nil, log, service.RepositoryDiscoveryConfig{}), logger: log}
	for _, tc := range []struct {
		err    error
		status int
		code   string
	}{
		{repository.ErrTaskCompletionVerifierReserved, http.StatusConflict, "task_completion_verifier_reserved"},
		{repository.ErrTaskCompletionSelfVerification, http.StatusForbidden, "task_completion_self_verification"},
	} {
		response := httptest.NewRecorder()
		ctx, _ := gin.CreateTestContext(response)
		ctx.Params = gin.Params{{Key: "id", Value: "task-1"}}
		ctx.Request = httptest.NewRequest(http.MethodPut, "/api/v1/tasks/task-1/completion-gate/criteria/ci/evidence", nil)
		handlers.handleCompletionGateError(ctx, tc.err)
		require.Equal(t, tc.status, response.Code)
		require.Contains(t, response.Body.String(), tc.code)
	}
}
