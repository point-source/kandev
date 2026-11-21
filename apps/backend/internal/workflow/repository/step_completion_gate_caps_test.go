package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-OFFICE-COMPLETION-GATES-007.1
func TestStepCompletionGateCapsRoundTrip(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Done")
	step.CompletionGateCaps = &models.CompletionGateCaps{MaxVerificationAttempts: 4, MaxReworkRounds: 2}
	if err := repo.UpdateStep(ctx, step); err != nil {
		t.Fatalf("update: %v", err)
	}
	got, err := repo.GetStep(ctx, step.ID)
	if err != nil || got.CompletionGateCaps == nil || *got.CompletionGateCaps != *step.CompletionGateCaps {
		t.Fatalf("caps = %+v, %v", got.CompletionGateCaps, err)
	}
	steps, err := repo.ListStepsByWorkflow(ctx, step.WorkflowID)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, listed := range steps {
		found = found || (listed.ID == step.ID && listed.CompletionGateCaps != nil)
	}
	if !found {
		t.Fatalf("listed caps missing among %d steps", len(steps))
	}
	step.CompletionGateCaps = nil
	if err := repo.UpdateStep(ctx, step); err != nil {
		t.Fatal(err)
	}
	if got, _ := repo.GetStep(ctx, step.ID); got.CompletionGateCaps != nil {
		t.Fatalf("cleared caps = %+v", got.CompletionGateCaps)
	}
}
