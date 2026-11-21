package repository

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/workflow/models"
)

// @covers AC-OFFICE-COMPLETION-GATES-004.1
func TestRecordStepDecisionKeepsPRHeads(t *testing.T) {
	repo := setupTestRepo(t)
	ctx := context.Background()
	step := newPhase2TestStep(t, repo, "Review")
	if err := repo.RecordStepDecision(ctx, &models.WorkflowStepDecision{
		TaskID: "task-1", StepID: step.ID, ParticipantID: "seat-1", Decision: "approved", PRHeads: "pr-1@h1",
	}); err != nil {
		t.Fatalf("record: %v", err)
	}
	rows, err := repo.ListStepDecisions(ctx, "task-1", step.ID)
	if err != nil || len(rows) != 1 || rows[0].PRHeads != "pr-1@h1" {
		t.Fatalf("rows = %+v, %v", rows, err)
	}
	active, err := repo.ListActiveTaskDecisions(ctx, "task-1")
	if err != nil || len(active) != 1 || active[0].PRHeads != "pr-1@h1" {
		t.Fatalf("active = %+v, %v", active, err)
	}
}
