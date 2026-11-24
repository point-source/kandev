package main

import (
	"net/http"
	"testing"
)

// @covers AC-OFFICE-COMPLETION-GATES-001.1
func TestTaskCreateSendsCompletionCriteria(t *testing.T) {
	captured := setupMockTransport(t, http.StatusCreated, `{"task_id":"t-1"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-run-token")
	code := runKandevCLI([]string{"task", "create", "--title", "Fix webhook",
		"--require", "github-check", "--require", "review", "--protect", "tests/**", "--rework-limit", "2",
		"--review-wait-limit", "2h"})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	criteria, ok := taskCreateBody(t, captured)["completion_criteria"].([]interface{})
	if !ok || len(criteria) != 2 {
		t.Fatalf("completion_criteria = %#v", criteria)
	}
	ci := criteria[0].(map[string]interface{})
	review := criteria[1].(map[string]interface{})
	if ci["kind"] != "check" || ci["runner"] != "github" || ci["rework_limit"] != float64(2) || ci["review_wait_limit"] != nil {
		t.Fatalf("check = %#v", ci)
	}
	if review["kind"] != "review" || review["protected_paths"] != nil || review["review_wait_limit"] != float64(120) {
		t.Fatalf("review = %#v", review)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.1
func TestTaskCreateRequiresAnApprovalWithoutAVerificationLimit(t *testing.T) {
	captured := setupMockTransport(t, http.StatusCreated, `{"task_id":"t-1"}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-run-token")
	code := runKandevCLI([]string{"task", "create", "--title", "Fix webhook",
		"--require", "github-check", "--require", "approval", "--verification-limit", "4", "--rework-limit", "2"})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	criteria, ok := taskCreateBody(t, captured)["completion_criteria"].([]interface{})
	if !ok || len(criteria) != 2 {
		t.Fatalf("completion_criteria = %#v", criteria)
	}
	approval := criteria[1].(map[string]interface{})
	if approval["kind"] != "approval" || approval["runner"] != nil || approval["verification_limit"] != nil ||
		approval["rework_limit"] != float64(2) {
		t.Fatalf("approval = %#v", approval)
	}
}

func TestTaskCreateRejectsUnknownCriterion(t *testing.T) {
	captured := setupMockTransport(t, http.StatusCreated, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-run-token")
	if code := runKandevCLI([]string{"task", "create", "--title", "x", "--require", "lint"}); code == 0 {
		t.Fatal("unknown criterion accepted")
	}
	if captured.Path != "" {
		t.Fatalf("request sent: %q", captured.Path)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-007.5
func TestTaskGateExtendPostsToTheCriterionRoute(t *testing.T) {
	captured := setupMockTransport(t, http.StatusOK, `{}`)
	t.Setenv("KANDEV_API_URL", "http://kandev.test")
	t.Setenv("KANDEV_API_KEY", "signed-run-token")
	if code := runKandevCLI([]string{"task", "gate", "extend", "--task", "t-1", "--criterion", "github_check", "--rework-limit", "5"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if captured.Path != "/api/v1/office/tasks/t-1/completion-gate/criteria/github_check/extend" {
		t.Fatalf("path = %q", captured.Path)
	}
	if body := taskCreateBody(t, captured); body["rework_limit"] != float64(5) {
		t.Fatalf("body = %#v", body)
	}
}
