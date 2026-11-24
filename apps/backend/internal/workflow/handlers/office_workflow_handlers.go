package handlers

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	"github.com/kandev/kandev/internal/workflow/service"
)

// officeWorkflowRefusedCode identifies a refused Office workflow write. The
// response's problems list carries one entry per reason.
const officeWorkflowRefusedCode = "office_workflow_refused"

func (h *Handlers) registerOfficeWorkflowHTTP(api *gin.RouterGroup) {
	api.GET("/workspaces/:id/office-workflow", h.httpGetOfficeWorkflow)
	api.PUT("/workspaces/:id/office-workflow", h.httpSelectOfficeWorkflow)
	api.POST("/workspaces/:id/office-workflows", h.httpCreateOfficeWorkflow)
	api.PUT("/workflows/:id/definition", h.httpSaveOfficeWorkflowDefinition)
}

func (h *Handlers) httpCreateOfficeWorkflow(c *gin.Context) {
	var def service.OfficeWorkflowDefinition
	if err := c.ShouldBindJSON(&def); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}
	def.WorkspaceID, def.WorkflowID = c.Param("id"), ""
	h.saveOfficeWorkflowDefinition(c, def, http.StatusCreated)
}

func (h *Handlers) httpSaveOfficeWorkflowDefinition(c *gin.Context) {
	var def service.OfficeWorkflowDefinition
	if err := c.ShouldBindJSON(&def); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request: " + err.Error()})
		return
	}
	def.WorkspaceID, def.WorkflowID = "", c.Param("id")
	h.saveOfficeWorkflowDefinition(c, def, http.StatusOK)
}

func (h *Handlers) saveOfficeWorkflowDefinition(c *gin.Context, def service.OfficeWorkflowDefinition, status int) {
	ctx := c.Request.Context()
	result, err := h.controller.SaveOfficeWorkflowDefinition(ctx, def)
	if err != nil {
		if errors.Is(err, repoerrors.ErrTaskVersionConflict) {
			c.JSON(http.StatusConflict, gin.H{"error": "workflow changed since it was loaded", "code": "workflow_changed"})
			return
		}
		h.writeOfficeWorkflowError(c, err, "failed to save office workflow")
		return
	}
	h.publishWorkflowStepEvents(ctx, events.WorkflowStepDeleted, result.Deleted)
	h.publishWorkflowStepEvents(ctx, events.WorkflowStepCreated, result.Created)
	h.publishWorkflowStepEvents(ctx, events.WorkflowStepUpdated, result.Updated)
	c.JSON(status, result)
}

func (h *Handlers) httpGetOfficeWorkflow(c *gin.Context) {
	state, err := h.controller.OfficeWorkflowState(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeOfficeWorkflowError(c, err, "failed to load office workflow")
		return
	}
	c.JSON(http.StatusOK, state)
}

type selectOfficeWorkflowRequest struct {
	WorkflowID string `json:"workflow_id"`
}

func (h *Handlers) httpSelectOfficeWorkflow(c *gin.Context) {
	var req selectOfficeWorkflowRequest
	if err := c.ShouldBindJSON(&req); err != nil || req.WorkflowID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "workflow_id is required"})
		return
	}
	ctx := c.Request.Context()
	workspaceID := c.Param("id")
	if err := h.controller.SelectOfficeWorkflow(ctx, workspaceID, req.WorkflowID); err != nil {
		h.writeOfficeWorkflowError(c, err, "failed to select office workflow")
		return
	}
	state, err := h.controller.OfficeWorkflowState(ctx, workspaceID)
	if err != nil {
		h.writeOfficeWorkflowError(c, err, "failed to load office workflow")
		return
	}
	c.JSON(http.StatusOK, state)
}

// writeOfficeWorkflowError maps Office workflow errors: refusals to 422 (or
// 409 for a write the target does not accept), denial to 403, invisible
// resources to 404.
func (h *Handlers) writeOfficeWorkflowError(c *gin.Context, err error, message string) {
	if refusal, ok := service.AsOfficeWorkflowRefusal(err); ok {
		status := http.StatusUnprocessableEntity
		if refusal.Conflict {
			status = http.StatusConflict
		}
		c.JSON(status, gin.H{"error": refusal.Error(), "code": officeWorkflowRefusedCode, "problems": refusal.Problems})
		return
	}
	if errors.Is(err, service.ErrWorkspaceManageDenied) {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	if errors.Is(err, service.ErrWorkflowReadOnly) || errors.Is(err, service.ErrWorkflowWorkspaceReadOnly) {
		c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
		return
	}
	if writeNotVisible(c, err, "Workspace not found") || writeExecutorMutationError(c, err) {
		return
	}
	if errors.Is(err, repoerrors.ErrWorkspaceNotFound) || errors.Is(err, repoerrors.ErrWorkflowNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"error": "Not found"})
		return
	}
	h.logger.Error(message, zap.Error(err))
	c.JSON(http.StatusInternalServerError, gin.H{"error": message})
}
