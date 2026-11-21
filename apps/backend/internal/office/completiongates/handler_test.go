package completiongates

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	officeagents "github.com/kandev/kandev/internal/office/agents"
)

func mountedHandler(t *testing.T, svc *Service, agent bool) *gin.Engine {
	t.Helper()
	gin.SetMode(gin.TestMode)
	engine := gin.New()
	group := engine.Group("/api/v1/office")
	if agent {
		group.Use(func(c *gin.Context) {
			c.Set("agent_claims", &officeagents.AgentClaims{AgentProfileID: "owner"})
		})
	}
	RegisterRoutes(group, svc, newTestLogger(t))
	return engine
}

func TestSettingsUpdateRefusesAgentCallers(t *testing.T) {
	svc, _ := newControlService(t, ciCriterion())
	body := `{"default_verification_limit":3,"default_rework_limit":3,"verification_limit_ceiling":10,"rework_limit_ceiling":10}`
	request := httptest.NewRequest(http.MethodPut, "/api/v1/office/workspaces/ws-1/completion-gates/settings", strings.NewReader(body))
	recorder := httptest.NewRecorder()
	mountedHandler(t, svc, true).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || !strings.Contains(recorder.Body.String(), "completion_control_requires_person") {
		t.Fatalf("agent settings update = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestSettingsUpdateReportsMalformedBodiesAsSettingsErrors(t *testing.T) {
	svc, _ := newControlService(t, ciCriterion())
	request := httptest.NewRequest(http.MethodPut, "/api/v1/office/workspaces/ws-1/completion-gates/settings", strings.NewReader(`{"bogus":1}`))
	recorder := httptest.NewRecorder()
	mountedHandler(t, svc, false).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusBadRequest || !strings.Contains(recorder.Body.String(), "invalid_completion_gate_settings") {
		t.Fatalf("malformed settings = %d %s", recorder.Code, recorder.Body.String())
	}
}

func TestEffectiveLimitsRoute(t *testing.T) {
	criterion := ciCriterion()
	criterion.Config.ReworkLimit, criterion.ReworkCount = 5, 2
	svc, _ := newControlService(t, criterion)
	request := httptest.NewRequest(http.MethodGet, "/api/v1/office/tasks/task-1/completion-gate/limits", nil)
	recorder := httptest.NewRecorder()
	mountedHandler(t, svc, false).ServeHTTP(recorder, request)
	var response struct {
		Limits []EffectiveLimits `json:"limits"`
	}
	if recorder.Code != http.StatusOK || json.Unmarshal(recorder.Body.Bytes(), &response) != nil {
		t.Fatalf("limits = %d %s", recorder.Code, recorder.Body.String())
	}
	if len(response.Limits) != 1 || response.Limits[0].ReworkLimit != 5 || response.Limits[0].VerificationLimit != DefaultLimit || response.Limits[0].ReworkCount != 2 {
		t.Fatalf("limits = %+v", response.Limits)
	}
}
