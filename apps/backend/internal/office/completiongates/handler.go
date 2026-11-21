package completiongates

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	officeagents "github.com/kandev/kandev/internal/office/agents"
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

const codeInvalidControl = "invalid_completion_control"

// Handler serves the human completion-gate controls.
type Handler struct {
	svc    *Service
	logger *logger.Logger
}

// RegisterRoutes mounts the routes on the Office route group.
func RegisterRoutes(router *gin.RouterGroup, svc *Service, log *logger.Logger) {
	if svc == nil {
		return
	}
	h := &Handler{svc: svc, logger: log}
	router.POST("/tasks/:id/completion-gate/criteria/:criterionId/protected-paths", h.protectedPaths)
	router.POST("/tasks/:id/completion-gate/criteria/:criterionId/extend", h.extend)
	router.GET("/tasks/:id/completion-gate/caps", h.caps)
	router.GET("/workspaces/:wsId/completion-gates/settings", h.getSettings)
	router.PUT("/workspaces/:wsId/completion-gates/settings", h.putSettings)
}

func (h *Handler) extend(c *gin.Context) {
	var request ExtendRequest
	if !decodeStrict(c, &request, codeInvalidControl) {
		return
	}
	var snapshot any
	var err error
	if claims := officeagents.ClaimsFromContext(c); claims != nil {
		snapshot, err = h.svc.ExtendAsAgent(c.Request.Context(), c.Param("id"), c.Param("criterionId"), claims.AgentProfileID, request)
	} else {
		snapshot, err = h.svc.ExtendAsPerson(c.Request.Context(), c.Param("id"), c.Param("criterionId"), request)
	}
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *Handler) caps(c *gin.Context) {
	caps, err := h.svc.EffectiveCaps(c.Request.Context(), c.Param("id"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"caps": caps})
}

func (h *Handler) getSettings(c *gin.Context) {
	settings, err := h.svc.GetSettings(c.Request.Context(), c.Param("wsId"))
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

func (h *Handler) putSettings(c *gin.Context) {
	if officeagents.ClaimsFromContext(c) != nil {
		h.writeError(c, ErrHumanRequired)
		return
	}
	var request officemodels.CompletionGateSettings
	if !decodeStrict(c, &request, "invalid_completion_gate_settings") {
		return
	}
	settings, err := h.svc.UpdateSettings(c.Request.Context(), c.Param("wsId"), request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, settings)
}

func decodeStrict(c *gin.Context, target any, code string) bool {
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": code})
		return false
	}
	return true
}

func (h *Handler) protectedPaths(c *gin.Context) {
	if officeagents.ClaimsFromContext(c) != nil {
		h.writeError(c, ErrHumanRequired)
		return
	}
	var request ProtectedPathsAction
	if !decodeStrict(c, &request, codeInvalidControl) {
		return
	}
	snapshot, err := h.svc.DecideProtectedPaths(c.Request.Context(), c.Param("id"), c.Param("criterionId"), request)
	if err != nil {
		h.writeError(c, err)
		return
	}
	c.JSON(http.StatusOK, snapshot)
}

func (h *Handler) writeError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, ErrHumanRequired):
		c.JSON(http.StatusForbidden, gin.H{"code": "completion_control_requires_person"})
	case errors.Is(err, ErrUnsupportedControl), errors.Is(err, ErrInvalidCriterion), errors.Is(err, ErrReasonRequired):
		c.JSON(http.StatusBadRequest, gin.H{"code": codeInvalidControl})
	case errors.Is(err, ErrInvalidSettings):
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_completion_gate_settings"})
	case errors.Is(err, ErrInvalidProtectedPath):
		c.JSON(http.StatusBadRequest, gin.H{"code": "invalid_protected_path"})
	case errors.Is(err, ErrCapAboveCeiling):
		c.JSON(http.StatusConflict, gin.H{"code": "completion_cap_above_ceiling"})
	case errors.Is(err, ErrNotCriterionOwner):
		c.JSON(http.StatusForbidden, gin.H{"code": "task_completion_self_verification"})
	case errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged):
		c.JSON(http.StatusConflict, gin.H{"code": "task_completion_evidence_changed"})
	case errors.Is(err, repoerrors.ErrTaskNotFound), taskservice.IsForbidden(err):
		c.JSON(http.StatusNotFound, gin.H{"code": "task_not_found"})
	default:
		h.logger.Error("completion gate control failed", zap.Error(err))
		c.JSON(http.StatusInternalServerError, gin.H{"code": "completion_control_failed"})
	}
}
