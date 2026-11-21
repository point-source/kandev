package completiongates

import (
	"context"
	"testing"
	"time"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

type fakeSettings struct {
	settings officemodels.CompletionGateSettings
}

func (f *fakeSettings) GetCompletionGateSettings(context.Context, string) (officemodels.CompletionGateSettings, bool, error) {
	return f.settings, true, nil
}

func (f *fakeSettings) SetCompletionGateSettings(_ context.Context, settings officemodels.CompletionGateSettings) error {
	f.settings = settings
	return nil
}

type fakeStepLimits struct{ attempts, rework, reviewWait int }

func (f fakeStepLimits) CompletingStepLimits(context.Context, string) (workflowmodels.CompletionGateLimits, error) {
	return workflowmodels.CompletionGateLimits{VerificationLimit: f.attempts, ReworkLimit: f.rework, ReviewWaitLimit: f.reviewWait}, nil
}

// @covers AC-OFFICE-COMPLETION-GATES-007.1
func TestLimitsResolveInPrecedenceOrderWithinTheCeiling(t *testing.T) {
	settings := &fakeSettings{settings: officemodels.CompletionGateSettings{DefaultVerificationLimit: 5, DefaultReworkLimit: 4, VerificationLimitCeiling: 8, ReworkLimitCeiling: 6}}
	criterion := ciCriterion()
	cases := []struct {
		name               string
		steps              fakeStepLimits
		config             models.TaskCompletionCriterionConfig
		wantAttempts, want int
	}{
		{"workspace default", fakeStepLimits{}, models.TaskCompletionCriterionConfig{}, 5, 4},
		{"step default", fakeStepLimits{attempts: 2, rework: 3}, models.TaskCompletionCriterionConfig{}, 2, 3},
		{"criterion value", fakeStepLimits{attempts: 2, rework: 3}, models.TaskCompletionCriterionConfig{VerificationLimit: 7, ReworkLimit: 1}, 7, 1},
		{"clamped", fakeStepLimits{}, models.TaskCompletionCriterionConfig{VerificationLimit: 50, ReworkLimit: 50}, 8, 6},
		{"confirmed above ceiling", fakeStepLimits{}, models.TaskCompletionCriterionConfig{VerificationLimit: 50, ReworkLimit: 50, LimitsConfirmedBy: "user"}, 50, 50},
	}
	for _, tc := range cases {
		policy := NewLimitsPolicy(fakeTasks{}, settings, tc.steps)
		criterion.Config = tc.config
		attempts, rework := policy.Resolve(context.Background(), "task-1", criterion)
		if attempts != tc.wantAttempts || rework != tc.want {
			t.Errorf("%s: got %d/%d, want %d/%d", tc.name, attempts, rework, tc.wantAttempts, tc.want)
		}
	}
	empty := NewLimitsPolicy(fakeTasks{}, &fakeSettings{}, fakeStepLimits{})
	if attempts, rework := empty.Resolve(context.Background(), "task-1", ciCriterion()); attempts != DefaultLimit || rework != DefaultLimit {
		t.Fatalf("shipped defaults = %d/%d", attempts, rework)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-007.1
func TestReviewWaitLimitIsOptionalAndResolvesWithoutACeiling(t *testing.T) {
	review := ciCriterion()
	review.Kind, review.Config = models.TaskCompletionCriterionReview, models.TaskCompletionCriterionConfig{}
	cases := []struct {
		name      string
		workspace int
		step      int
		criterion int
		want      time.Duration
	}{
		{"unset waits indefinitely", 0, 0, 0, 0},
		{"workspace default", 90, 0, 0, 90 * time.Minute},
		{"step default", 90, 30, 0, 30 * time.Minute},
		{"criterion value, no ceiling", 90, 30, 5000, 5000 * time.Minute},
	}
	for _, tc := range cases {
		settings := &fakeSettings{settings: officemodels.CompletionGateSettings{DefaultReviewWaitLimit: tc.workspace}}
		policy := NewLimitsPolicy(fakeTasks{}, settings, fakeStepLimits{reviewWait: tc.step})
		review.Config.ReviewWaitLimit = tc.criterion
		if got := policy.ReviewWait(context.Background(), "task-1", review); got != tc.want {
			t.Errorf("%s: review wait = %v, want %v", tc.name, got, tc.want)
		}
	}
}
