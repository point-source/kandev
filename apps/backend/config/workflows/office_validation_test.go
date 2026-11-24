package workflows

import (
	"testing"

	wfmodels "github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-OFFICE-WORKFLOW-SELECT-005.7
func TestOfficeDefaultTemplatePassesOfficeValidation(t *testing.T) {
	templates, err := LoadTemplates()
	if err != nil {
		t.Fatalf("LoadTemplates: %v", err)
	}
	for _, tmpl := range templates {
		if !tmpl.Office {
			continue
		}
		steps := make([]*wfmodels.WorkflowStep, 0, len(tmpl.Steps))
		for _, def := range tmpl.Steps {
			steps = append(steps, &wfmodels.WorkflowStep{
				ID: def.ID, Name: def.Name, Position: def.Position, Events: def.Events,
				IsStartStep: def.IsStartStep, StageType: def.StageType,
				AutoAdvanceRequiresSignal: def.AutoAdvanceRequiresSignal,
				CompleteTaskOnEnter:       def.CompleteTaskOnEnter,
			})
		}
		if problems := wfmodels.ValidateOfficeWorkflow(steps, func(string) bool { return true }); len(problems) != 0 {
			t.Errorf("template %q problems = %+v, want none", tmpl.ID, problems)
		}
	}
	found := false
	for _, tmpl := range templates {
		found = found || (tmpl.ID == "office-default" && tmpl.Office)
	}
	if !found {
		t.Fatal("office-default template must declare office: true")
	}
}
