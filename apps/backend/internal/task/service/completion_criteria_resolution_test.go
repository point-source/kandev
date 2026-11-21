package service

import (
	"context"
	"errors"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

type stubCriteriaResolver struct {
	input models.CompletionCriteriaResolution
	out   []models.TaskCompletionCriterion
	err   error
}

func (s *stubCriteriaResolver) ResolveCreateCriteria(_ context.Context, input models.CompletionCriteriaResolution) ([]models.TaskCompletionCriterion, error) {
	s.input = input
	return s.out, s.err
}

// @covers AC-OFFICE-COMPLETION-GATES-001.1, AC-OFFICE-COMPLETION-GATES-001.4
func TestOfficeCreateResolvesCompletionCriteria(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wf := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "gate-resolve")
	ensureTaskPRTable(t, repo)
	resolver := &stubCriteriaResolver{out: []models.TaskCompletionCriterion{{
		ID: "ci_at_head", Kind: models.TaskCompletionCriterionCIAtHead, OwnerKind: "agent", OwnerID: "ceo",
	}}}
	svc.SetCompletionCriteriaResolver(resolver)
	requested := []models.TaskCompletionCriterionRequest{{Kind: models.TaskCompletionCriterionCIAtHead}}
	created, err := svc.CreateTask(ctx, &CreateTaskRequest{
		WorkspaceID: "gate-resolve", WorkflowID: wf, Title: "Office", Origin: models.TaskOriginAgentCreated,
		CompletionCriteriaRequest: &requested,
		CompletionCriteriaCreator: models.CompletionCriteriaCreator{Kind: models.CompletionCriteriaCreatorAgent, ID: "ceo"},
	})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if resolver.input.Creator.ID != "ceo" || resolver.input.Requested == nil || !resolver.input.IsOffice {
		t.Fatalf("resolver input = %+v", resolver.input)
	}
	gate, err := svc.GetTaskCompletionGate(ctx, created.Task.ID)
	if err != nil || len(gate.Criteria) != 1 || gate.Criteria[0].OwnerID != "ceo" {
		t.Fatalf("gate = %+v, %v", gate, err)
	}

	resolver.out = nil
	plain, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "gate-resolve", WorkflowID: wf, Title: "Plain"})
	if err != nil {
		t.Fatalf("non-office create: %v", err)
	}
	if gate, _ := svc.GetTaskCompletionGate(ctx, plain.Task.ID); len(gate.Criteria) != 0 {
		t.Fatalf("non-office task got criteria: %+v", gate.Criteria)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-001.3
func TestCompletionCriteriaRefusalsCreateNothing(t *testing.T) {
	svc, _, repo := createTestService(t)
	ctx := context.Background()
	wf := seedWorkspaceAndWorkflowForCreate(t, ctx, repo, "gate-refuse")
	requested := []models.TaskCompletionCriterionRequest{{Kind: models.TaskCompletionCriterionCIAtHead}}
	_, err := svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "gate-refuse", WorkflowID: wf, Title: "Plain", CompletionCriteriaRequest: &requested})
	if code, ok := IsCompletionCriteriaRefusal(err); !ok || code != "completion_criteria_requires_office_task" {
		t.Fatalf("non-office criteria err = %v", err)
	}
	svc.SetCompletionCriteriaResolver(&stubCriteriaResolver{err: &CompletionCriteriaRefusal{Code: "completion_criteria_requires_repository"}})
	_, err = svc.CreateTask(ctx, &CreateTaskRequest{WorkspaceID: "gate-refuse", WorkflowID: wf, Title: "Office", Origin: models.TaskOriginAgentCreated})
	var refusal *CompletionCriteriaRefusal
	if !errors.As(err, &refusal) || refusal.Code != "completion_criteria_requires_repository" {
		t.Fatalf("resolver refusal err = %v", err)
	}
	if tasks, _ := repo.ListTasks(ctx, wf); len(tasks) != 0 {
		t.Fatalf("tasks after refusals = %d", len(tasks))
	}
}
