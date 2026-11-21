package completiongates

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

// @covers AC-OFFICE-COMPLETION-GATES-001.1
func TestResolveCreateCriteriaNamesTheCheckRunner(t *testing.T) {
	svc := newCreateService(t, nil, true)
	check := models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionCheck, Runner: models.TaskCompletionCheckRunnerGitHub,
		ReviewWaitLimit: 0}
	review := models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionReview, ReviewWaitLimit: 120}
	criteria, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorHuman, check, review))
	if err != nil || len(criteria) != 2 {
		t.Fatalf("criteria = %+v, %v", criteria, err)
	}
	if criteria[0].ID != "github_check" || criteria[0].Config.Runner != models.TaskCompletionCheckRunnerGitHub {
		t.Fatalf("check = %+v", criteria[0])
	}
	if criteria[1].ID != "review" || criteria[1].Config.ReviewWaitLimit != 120 {
		t.Fatalf("review = %+v", criteria[1])
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-001.1
func TestResolveCreateCriteriaRefusesAMissingOrMisplacedRunner(t *testing.T) {
	svc := newCreateService(t, nil, true)
	refused := []models.TaskCompletionCriterionRequest{
		{Kind: models.TaskCompletionCriterionCheck},
		{Kind: models.TaskCompletionCriterionCheck, Runner: "jenkins"},
		{Kind: models.TaskCompletionCriterionReview, Runner: models.TaskCompletionCheckRunnerGitHub},
		{Kind: models.TaskCompletionCriterionCheck, Runner: models.TaskCompletionCheckRunnerGitHub, ReviewWaitLimit: 30},
	}
	for _, request := range refused {
		if _, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorHuman, request)); err == nil {
			t.Errorf("request %+v was accepted", request)
		}
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-001.3
func TestCriteriaEditsNeedAReviewStageForAReview(t *testing.T) {
	review := []models.TaskCompletionCriterion{{ID: "review", Kind: models.TaskCompletionCriterionReview}}
	task := &models.Task{ID: "task-1", WorkflowID: "wf"}
	err := newCreateService(t, nil, false).ValidateCriteriaChange(context.Background(), task, review)
	if refusalCode(err) != "completion_criteria_requires_review_stage" {
		t.Fatalf("err = %v", err)
	}
	if err := newCreateService(t, nil, true).ValidateCriteriaChange(context.Background(), task, review); err != nil {
		t.Fatalf("review stage present: %v", err)
	}
}
