package service

import (
	"strings"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-OFFICE-COMPLETION-GATES-006.1
func TestPromptListsUnmetCompletionCriteria(t *testing.T) {
	gate := &models.TaskCompletionGateSnapshot{
		Blocked: true,
		Criteria: []models.TaskCompletionCriterion{
			{ID: "ci", Kind: "ci_at_head", Description: "CI passes at the PR head commit", Result: "fail",
				ResultReason: "checks_failed", ResultDetail: []string{"lint", "unit"}},
			{ID: "review", Kind: "review_at_head", Description: "Review approved at the PR head commit", Result: "pass"},
		},
		Blockers: []models.TaskCompletionBlocker{{CriterionID: "ci", Reason: "unverified"}},
	}
	prompt := BuildPrompt(&PromptContext{Reason: RunReasonTaskAssigned, TaskID: "t1", CompletionGate: gate})
	if !strings.Contains(prompt, "## Completion criteria") || !strings.Contains(prompt, "checks_failed") ||
		!strings.Contains(prompt, "lint, unit") || strings.Contains(prompt, "Review approved at the PR head commit") {
		t.Fatalf("prompt section missing or wrong:\n%s", prompt)
	}
	if strings.Contains(BuildPrompt(&PromptContext{Reason: RunReasonTaskAssigned, TaskID: "t1"}), "## Completion criteria") {
		t.Fatal("ungated prompt rendered the section")
	}
}
