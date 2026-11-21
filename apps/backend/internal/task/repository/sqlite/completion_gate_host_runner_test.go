package sqlite

import (
	"context"
	"testing"

	"github.com/kandev/kandev/internal/task/models"
)

func TestHostCriteriaNeedTheirRunnerAndOwnLimits(t *testing.T) {
	review := ciCriterion()
	review.ID, review.Kind, review.Config = "review", models.TaskCompletionCriterionReview, models.TaskCompletionCriterionConfig{}
	noRunner := ciCriterion()
	noRunner.Config.Runner = ""
	unknownRunner := ciCriterion()
	unknownRunner.Config.Runner = "jenkins"
	reviewWithRunner := review
	reviewWithRunner.Config.Runner = models.TaskCompletionCheckRunnerGitHub
	checkWithWait := ciCriterion()
	checkWithWait.Config.ReviewWaitLimit = 30
	for name, bad := range map[string]models.TaskCompletionCriterion{
		"check without runner": noRunner, "unknown runner": unknownRunner,
		"review with runner": reviewWithRunner, "check with review wait": checkWithWait,
	} {
		if err := validateCompletionCriteriaForTask(hostGateTaskID, []models.TaskCompletionCriterion{bad}); err == nil {
			t.Errorf("%s was accepted", name)
		}
	}
	review.Config.ReviewWaitLimit = 120
	if err := validateCompletionCriteriaForTask(hostGateTaskID, []models.TaskCompletionCriterion{ciCriterion(), review}); err != nil {
		t.Fatalf("valid criteria refused: %v", err)
	}
	_ = context.Background
}
