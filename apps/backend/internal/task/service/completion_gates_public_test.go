package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

func forgedCriterion() models.TaskCompletionCriterion {
	return models.TaskCompletionCriterion{
		ID: "ci", Kind: models.TaskCompletionCriterionCheck, OwnerKind: "agent", OwnerID: "ceo",
		Config: models.TaskCompletionCriterionConfig{Runner: models.TaskCompletionCheckRunnerGitHub, ReworkLimit: 50, LimitsConfirmedBy: "someone-else"},
	}
}

func TestPublicCriteriaTakeOwnerAndLimitsConfirmationFromTheCaller(t *testing.T) {
	clean := publicCompletionCriteria("task-1", []models.TaskCompletionCriterion{forgedCriterion()}, "agent-7", false)
	if clean[0].OwnerKind != "" || clean[0].OwnerID != "" || clean[0].Config.LimitsConfirmedBy != "" {
		t.Fatalf("non-person criteria kept forged fields: %+v", clean[0])
	}
	confirmed := publicCompletionCriteria("task-1", []models.TaskCompletionCriterion{forgedCriterion()}, "user-1", true)
	if confirmed[0].Config.LimitsConfirmedBy != "user-1" {
		t.Fatalf("person confirmation = %q", confirmed[0].Config.LimitsConfirmedBy)
	}
}

func TestSetCriteriaStoresTheActingCallerAsOwner(t *testing.T) {
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1", Role: authn.RoleAdmin})
	svc, _, repo := createTestService(t)
	seedMoveWorkflows(t, ctx, repo)
	createMoveTask(t, ctx, repo, "task-owned", "wf-source", "step-source", nil)
	ensureTaskPRTable(t, repo)
	snapshot, err := svc.SetTaskCompletionCriteria(ctx, "task-owned", SetTaskCompletionCriteriaRequest{
		Criteria: []models.TaskCompletionCriterion{forgedCriterion()},
	})
	if err != nil {
		t.Fatalf("set criteria: %v", err)
	}
	criterion := snapshot.Criteria[0]
	if criterion.OwnerKind != completionGateActorHuman || criterion.OwnerID != "user-1" || criterion.Config.LimitsConfirmedBy != "user-1" {
		t.Fatalf("stored criterion = %+v", criterion)
	}
}
