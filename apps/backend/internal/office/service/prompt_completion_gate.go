package service

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/task/models"
)

// CompletionGateReader supplies a task's completion gate for the prompt.
type CompletionGateReader interface {
	GetTaskCompletionGate(ctx context.Context, taskID string) (*models.TaskCompletionGateSnapshot, error)
}

// SetCompletionGateReader wires the completion-criteria prompt section.
func (si *SchedulerIntegration) SetCompletionGateReader(reader CompletionGateReader) {
	si.completionGates = reader
}

func (si *SchedulerIntegration) enrichCompletionGate(ctx context.Context, pc *PromptContext, taskID string) {
	if si.completionGates == nil || taskID == "" {
		return
	}
	snapshot, err := si.completionGates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		si.logger.Debug("load completion gate for prompt failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	pc.CompletionGate = snapshot
}

// appendCompletionGateSection lists unmet criteria so a woken worker knows
// why the task is not done.
func appendCompletionGateSection(prompt string, snapshot *models.TaskCompletionGateSnapshot) string {
	if snapshot == nil || !snapshot.Blocked || len(snapshot.Criteria) == 0 {
		return prompt
	}
	blocked := make(map[string]string, len(snapshot.Blockers))
	for _, blocker := range snapshot.Blockers {
		blocked[blocker.CriterionID] = blocker.Reason
	}
	var b strings.Builder
	b.WriteString("\n\n## Completion criteria\n\nThis task cannot complete until these criteria pass. Kandev checks them itself; you cannot verify or change them.\n")
	for _, criterion := range snapshot.Criteria {
		blocker, unmet := blocked[criterion.ID]
		if !unmet {
			continue
		}
		fmt.Fprintf(&b, "\n- %s (%s): %s", sanitizePromptField(criterion.Description), criterion.Kind, completionStatusLine(criterion, blocker))
		if len(criterion.ResultDetail) > 0 {
			fmt.Fprintf(&b, "\n  - %s", sanitizePromptField(strings.Join(criterion.ResultDetail, ", ")))
		}
	}
	return prompt + b.String()
}

func completionStatusLine(criterion models.TaskCompletionCriterion, blocker string) string {
	parts := []string{blocker}
	if criterion.Result != "" {
		parts = append(parts, "result "+criterion.Result)
	}
	if criterion.ResultReason != "" {
		parts = append(parts, criterion.ResultReason)
	}
	if criterion.HeldReason != "" {
		parts = append(parts, "held: "+criterion.HeldReason)
	}
	return strings.Join(parts, ", ")
}

// buildCompletionGateHeldPrompt asks a criterion owner to resolve a hold.
func buildCompletionGateHeldPrompt(pc *PromptContext) string {
	var b strings.Builder
	fmt.Fprintf(&b, "A completion criterion on task %s is held and needs your decision.\n\n", taskRef(pc))
	fmt.Fprintf(&b, "Criterion: %s\nReason: %s\n", sanitizePromptField(pc.CompletionGateCriterionID), sanitizePromptField(pc.CompletionGateHeldReason))
	if pc.CompletionGateHistory != "" {
		fmt.Fprintf(&b, "\nRecent history:\n%s\n", capRunes(pc.CompletionGateHistory, 2000))
	}
	b.WriteString("\nThe assignee will not run until the hold is released. Choose one: raise the cap within the workspace ceiling with " +
		"`$KANDEV_CLI kandev task gate extend --task <id> --criterion <id> --rework <n>` (or `--attempts <n>`), " +
		"reassign the task, comment with guidance, or ask a person to decide in the Inbox.")
	return b.String()
}
