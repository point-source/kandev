package service

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/task/models"
)

type refusingCriteriaPolicy struct{ checked []string }

func (p *refusingCriteriaPolicy) ResolveCreateCriteria(context.Context, models.CompletionCriteriaResolution) ([]models.TaskCompletionCriterion, error) {
	return nil, nil
}

func (p *refusingCriteriaPolicy) ValidateCriteriaChange(_ context.Context, task *models.Task, criteria []models.TaskCompletionCriterion) error {
	p.checked = append(p.checked, task.ID)
	for _, criterion := range criteria {
		if criterion.Kind == models.TaskCompletionCriterionReview {
			return &CompletionCriteriaRefusal{Code: "completion_criteria_requires_review_stage"}
		}
	}
	return nil
}

func reviewOnly() []models.TaskCompletionCriterion {
	return []models.TaskCompletionCriterion{{ID: "review", Kind: models.TaskCompletionCriterionReview}}
}

// @covers AC-OFFICE-COMPLETION-GATES-001.3
func TestEditingCriteriaAppliesTheOwnersRefusals(t *testing.T) {
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1", Role: authn.RoleAdmin})
	svc, _, repo := createTestService(t)
	seedMoveWorkflows(t, ctx, repo)
	createMoveTask(t, ctx, repo, "task-edited", "wf-source", "step-source", nil)
	ensureTaskPRTable(t, repo)
	policy := &refusingCriteriaPolicy{}
	svc.SetCompletionCriteriaResolver(policy)
	_, err := svc.SetTaskCompletionCriteria(ctx, "task-edited", SetTaskCompletionCriteriaRequest{Criteria: reviewOnly()})
	if code, ok := IsCompletionCriteriaRefusal(err); !ok || code != "completion_criteria_requires_review_stage" {
		t.Fatalf("edit err = %v", err)
	}
	_, err = svc.SetTaskCompletionCriteriaExact(ctx, "task-edited", ExactTaskCompletionCriteriaRequest{ActorID: "plugin-1", Criteria: reviewOnly()})
	if code, ok := IsCompletionCriteriaRefusal(err); !ok || code != "completion_criteria_requires_review_stage" {
		t.Fatalf("plugin err = %v", err)
	}
	if len(policy.checked) != 2 {
		t.Fatalf("policy checked %v", policy.checked)
	}
}
