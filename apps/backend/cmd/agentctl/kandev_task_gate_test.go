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
		"--require", "ci-at-head", "--require", "review-at-head", "--protect", "tests/**", "--max-rework", "2"})
	if code != 0 {
		t.Fatalf("exit = %d", code)
	}
	criteria, ok := taskCreateBody(t, captured)["completion_criteria"].([]interface{})
	if !ok || len(criteria) != 2 {
		t.Fatalf("completion_criteria = %#v", criteria)
	}
	ci := criteria[0].(map[string]interface{})
	review := criteria[1].(map[string]interface{})
	if ci["kind"] != "ci_at_head" || ci["max_rework_rounds"] != float64(2) || review["protected_paths"] != nil {
		t.Fatalf("criteria = %#v", criteria)
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
	if code := runKandevCLI([]string{"task", "gate", "extend", "--task", "t-1", "--criterion", "ci_at_head", "--rework", "5"}); code != 0 {
		t.Fatalf("exit = %d", code)
	}
	if captured.Path != "/api/v1/office/tasks/t-1/completion-gate/criteria/ci_at_head/extend" {
		t.Fatalf("path = %q", captured.Path)
	}
	if body := taskCreateBody(t, captured); body["max_rework_rounds"] != float64(5) {
		t.Fatalf("body = %#v", body)
	}
}
