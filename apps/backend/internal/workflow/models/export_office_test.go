package models

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	taskmodels "github.com/kandev/kandev/internal/task/models"
)

// @covers AC-OFFICE-WORKFLOW-SELECT-007.1
func TestExportMarksOfficeWorkflows(t *testing.T) {
	office := &taskmodels.Workflow{ID: "wf-office", Name: "Office copy", IsOffice: true}
	kanban := &taskmodels.Workflow{ID: "wf-kanban", Name: "Kanban"}
	steps := validOfficeSteps()
	for _, s := range steps {
		s.WorkflowID = "wf-office"
	}
	export, err := BuildWorkflowExportWithError(
		[]*taskmodels.Workflow{office, kanban},
		map[string][]*WorkflowStep{"wf-office": steps}, nil)
	if err != nil {
		t.Fatalf("BuildWorkflowExportWithError: %v", err)
	}
	if !export.Workflows[0].Office || export.Workflows[1].Office {
		t.Fatalf("office flags = %v, %v; want true, false", export.Workflows[0].Office, export.Workflows[1].Office)
	}
	if export.Workflows[0].Steps[2].StageType != StageTypeReview {
		t.Fatalf("stage type not exported: %q", export.Workflows[0].Steps[2].StageType)
	}
	raw, err := yaml.Marshal(export)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	text := string(raw)
	if !strings.Contains(text, "office: true") || strings.Count(text, "office: true") != 1 {
		t.Fatalf("export YAML must mark only the Office workflow:\n%s", text)
	}
	for _, runtimeOnly := range []string{"participants:", "decisions:"} {
		if strings.Contains(text, runtimeOnly) {
			t.Errorf("export YAML contains runtime-only %q", runtimeOnly)
		}
	}
}
