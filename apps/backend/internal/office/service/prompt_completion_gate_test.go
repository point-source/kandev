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
			{ID: "ci", Kind: "check", Description: "CI passes at the PR head commit", Result: "fail",
				ResultReason: "checks_failed", ResultDetail: []string{"lint", "unit"}},
			{ID: "review", Kind: "review", Description: "Review approved at the PR head commit", Result: "pass"},
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

// @covers AC-OFFICE-COMPLETION-GATES-007.4, AC-OFFICE-COMPLETION-GATES-009.2
func TestHeldPromptNamesTheChoicesForTheHoldKind(t *testing.T) {
	limit := buildCompletionGateHeldPrompt(&PromptContext{CompletionGateCriterionID: "github_check", CompletionGateHoldKind: "rework_limit_reached"})
	if !strings.Contains(limit, "--rework-limit") || strings.Contains(limit, "reviewer agent") {
		t.Fatalf("limit prompt = %q", limit)
	}
	reviewer := buildCompletionGateHeldPrompt(&PromptContext{CompletionGateCriterionID: "review", CompletionGateHoldKind: "no_reviewer_available"})
	if !strings.Contains(reviewer, "reviewer agent") || strings.Contains(reviewer, "--rework-limit") {
		t.Fatalf("reviewer prompt = %q", reviewer)
	}
}
