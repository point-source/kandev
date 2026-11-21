package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-OFFICE-COMPLETION-GATES-007.1
func TestStepCompletionGateLimitsRoundTrip(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Done")
	step.CompletionGateLimits = &models.CompletionGateLimits{VerificationLimit: 4, ReworkLimit: 2}
	if err := repo.UpdateStep(ctx, step); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := repo.GetStep(ctx, step.ID)
	if err != nil || got.CompletionGateLimits == nil || *got.CompletionGateLimits != *step.CompletionGateLimits {
		t.Fatalf("limits = %+v, %v", got.CompletionGateLimits, err)
	}
	steps, err := repo.ListStepsByWorkflow(ctx, step.WorkflowID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, listed := range steps {
		found = found || (listed.ID == step.ID && listed.CompletionGateLimits != nil)
	}
	if !found {
		t.Fatalf("listed limits missing among %d steps", len(steps))
	}
	step.CompletionGateLimits = nil
	if err := repo.UpdateStep(ctx, step); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetStep(ctx, step.ID); got.CompletionGateLimits != nil {
		t.Fatalf("cleared limits = %+v", got.CompletionGateLimits)
	}
}
