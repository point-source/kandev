package completiongates

import (
	"context"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

// DefaultCeiling is the shipped workspace ceiling for both caps.
const DefaultCeiling = 10

// SettingsStore persists workspace completion-gate settings.
type SettingsStore interface {
	GetCompletionGateSettings(ctx context.Context, workspaceID string) (officemodels.CompletionGateSettings, bool, error)
	SetCompletionGateSettings(ctx context.Context, settings officemodels.CompletionGateSettings) error
}

// StepCapsReader returns the cap defaults of a workflow's completing step.
type StepCapsReader interface {
	CompletingStepCaps(ctx context.Context, workflowID string) (attempts, rework int, err error)
}

// CapsPolicy resolves effective caps: criterion, then completing step, then
// workspace default, then the shipped default, within the workspace ceiling
// unless a person confirmed the criterion value.
type CapsPolicy struct {
	tasks    TaskReader
	settings SettingsStore
	steps    StepCapsReader
}

// NewCapsPolicy creates the cap resolver.
func NewCapsPolicy(tasks TaskReader, settings SettingsStore, steps StepCapsReader) *CapsPolicy {
	return &CapsPolicy{tasks: tasks, settings: settings, steps: steps}
}

// Resolve returns the effective attempt and rework caps of a criterion.
func (p *CapsPolicy) Resolve(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion) (int, int) {
	var stepAttempts, stepRework int
	settings := officemodels.CompletionGateSettings{}
	if task, err := p.tasks.GetTask(ctx, taskID); err == nil {
		settings = p.workspaceSettings(ctx, task.WorkspaceID)
		if p.steps != nil {
			stepAttempts, stepRework, _ = p.steps.CompletingStepCaps(ctx, task.WorkflowID)
		}
	}
	confirmed := criterion.Config.CapsConfirmedBy != ""
	attempts := effectiveCap(confirmed, ceilingOrDefault(settings.CeilingAttempts),
		criterion.Config.MaxVerificationAttempts, stepAttempts, settings.DefaultMaxAttempts)
	rework := effectiveCap(confirmed, ceilingOrDefault(settings.CeilingRework),
		criterion.Config.MaxReworkRounds, stepRework, settings.DefaultMaxRework)
	return attempts, rework
}

// Ceilings returns the workspace ceilings.
func (p *CapsPolicy) Ceilings(ctx context.Context, workspaceID string) (int, int) {
	settings := p.workspaceSettings(ctx, workspaceID)
	return ceilingOrDefault(settings.CeilingAttempts), ceilingOrDefault(settings.CeilingRework)
}

func (p *CapsPolicy) workspaceSettings(ctx context.Context, workspaceID string) officemodels.CompletionGateSettings {
	if p.settings == nil || workspaceID == "" {
		return officemodels.CompletionGateSettings{}
	}
	settings, _, err := p.settings.GetCompletionGateSettings(ctx, workspaceID)
	if err != nil {
		return officemodels.CompletionGateSettings{}
	}
	return settings
}

func effectiveCap(confirmed bool, ceiling int, criterionValue int, fallbacks ...int) int {
	if criterionValue > 0 {
		if confirmed || criterionValue <= ceiling {
			return criterionValue
		}
		return ceiling
	}
	for _, value := range fallbacks {
		if value > 0 {
			return min(value, ceiling)
		}
	}
	return min(DefaultCap, ceiling)
}

func ceilingOrDefault(value int) int {
	if value > 0 {
		return value
	}
	return DefaultCeiling
}
