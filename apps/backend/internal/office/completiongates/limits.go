package completiongates

import (
	"context"
	"time"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// DefaultCeiling is the shipped workspace ceiling for both counted limits.
const DefaultCeiling = 10

// MaxReviewWaitLimit bounds a review wait limit, in minutes (30 days).
const MaxReviewWaitLimit = 30 * 24 * 60

// SettingsStore persists workspace completion-gate settings.
type SettingsStore interface {
	GetCompletionGateSettings(ctx context.Context, workspaceID string) (officemodels.CompletionGateSettings, bool, error)
	SetCompletionGateSettings(ctx context.Context, settings officemodels.CompletionGateSettings) error
}

// StepLimitsReader returns the limit defaults of a workflow's completing step.
type StepLimitsReader interface {
	CompletingStepLimits(ctx context.Context, workflowID string) (workflowmodels.CompletionGateLimits, error)
}

// LimitsPolicy resolves effective limits: criterion, then completing step, then
// workspace default, then the shipped default, within the workspace ceiling
// unless a person confirmed the criterion value.
type LimitsPolicy struct {
	tasks    TaskReader
	settings SettingsStore
	steps    StepLimitsReader
}

// NewLimitsPolicy creates the limit resolver.
func NewLimitsPolicy(tasks TaskReader, settings SettingsStore, steps StepLimitsReader) *LimitsPolicy {
	return &LimitsPolicy{tasks: tasks, settings: settings, steps: steps}
}

// Resolve returns the effective attempt and rework limits of a criterion.
func (p *LimitsPolicy) Resolve(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion) (int, int) {
	settings, step := p.sources(ctx, taskID)
	confirmed := criterion.Config.LimitsConfirmedBy != ""
	attempts := effectiveLimit(confirmed, ceilingOrDefault(settings.VerificationLimitCeiling),
		criterion.Config.VerificationLimit, step.VerificationLimit, settings.DefaultVerificationLimit)
	rework := effectiveLimit(confirmed, ceilingOrDefault(settings.ReworkLimitCeiling),
		criterion.Config.ReworkLimit, step.ReworkLimit, settings.DefaultReworkLimit)
	return attempts, rework
}

// ReviewWait returns how long a parked reviewer seat run may wait before the
// review holds: criterion, then completing step, then workspace default. Zero
// means wait indefinitely. It has no ceiling because waiting starts no runs.
func (p *LimitsPolicy) ReviewWait(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion) time.Duration {
	settings, step := p.sources(ctx, taskID)
	for _, minutes := range []int{criterion.Config.ReviewWaitLimit, step.ReviewWaitLimit, settings.DefaultReviewWaitLimit} {
		if minutes > 0 {
			return time.Duration(minutes) * time.Minute
		}
	}
	return 0
}

func (p *LimitsPolicy) sources(ctx context.Context, taskID string) (officemodels.CompletionGateSettings, workflowmodels.CompletionGateLimits) {
	task, err := p.tasks.GetTask(ctx, taskID)
	if err != nil {
		return officemodels.CompletionGateSettings{}, workflowmodels.CompletionGateLimits{}
	}
	var step workflowmodels.CompletionGateLimits
	if p.steps != nil {
		step, _ = p.steps.CompletingStepLimits(ctx, task.WorkflowID)
	}
	return p.workspaceSettings(ctx, task.WorkspaceID), step
}

// Ceilings returns the workspace ceilings.
func (p *LimitsPolicy) Ceilings(ctx context.Context, workspaceID string) (int, int) {
	settings := p.workspaceSettings(ctx, workspaceID)
	return ceilingOrDefault(settings.VerificationLimitCeiling), ceilingOrDefault(settings.ReworkLimitCeiling)
}

func (p *LimitsPolicy) workspaceSettings(ctx context.Context, workspaceID string) officemodels.CompletionGateSettings {
	if p.settings == nil || workspaceID == "" {
		return officemodels.CompletionGateSettings{}
	}
	settings, _, err := p.settings.GetCompletionGateSettings(ctx, workspaceID)
	if err != nil {
		return officemodels.CompletionGateSettings{}
	}
	return settings
}

func effectiveLimit(confirmed bool, ceiling int, criterionValue int, fallbacks ...int) int {
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
	return min(DefaultLimit, ceiling)
}

func ceilingOrDefault(value int) int {
	if value > 0 {
		return value
	}
	return DefaultCeiling
}
