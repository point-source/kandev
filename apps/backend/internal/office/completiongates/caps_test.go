package completiongates

import (
	"context"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
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

type fakeStepCaps struct{ attempts, rework int }

func (f fakeStepCaps) CompletingStepCaps(context.Context, string) (int, int, error) {
	return f.attempts, f.rework, nil
}

// @covers AC-OFFICE-COMPLETION-GATES-007.1
func TestCapsResolveInPrecedenceOrderWithinTheCeiling(t *testing.T) {
	settings := &fakeSettings{settings: officemodels.CompletionGateSettings{DefaultMaxAttempts: 5, DefaultMaxRework: 4, CeilingAttempts: 8, CeilingRework: 6}}
	criterion := ciCriterion()
	cases := []struct {
		name               string
		steps              fakeStepCaps
		config             models.TaskCompletionCriterionConfig
		wantAttempts, want int
	}{
		{"workspace default", fakeStepCaps{}, models.TaskCompletionCriterionConfig{}, 5, 4},
		{"step default", fakeStepCaps{attempts: 2, rework: 3}, models.TaskCompletionCriterionConfig{}, 2, 3},
		{"criterion value", fakeStepCaps{attempts: 2, rework: 3}, models.TaskCompletionCriterionConfig{MaxVerificationAttempts: 7, MaxReworkRounds: 1}, 7, 1},
		{"clamped", fakeStepCaps{}, models.TaskCompletionCriterionConfig{MaxVerificationAttempts: 50, MaxReworkRounds: 50}, 8, 6},
		{"confirmed above ceiling", fakeStepCaps{}, models.TaskCompletionCriterionConfig{MaxVerificationAttempts: 50, MaxReworkRounds: 50, CapsConfirmedBy: "user"}, 50, 50},
	}
	for _, tc := range cases {
		policy := NewCapsPolicy(fakeTasks{}, settings, tc.steps)
		criterion.Config = tc.config
		attempts, rework := policy.Resolve(context.Background(), "task-1", criterion)
		if attempts != tc.wantAttempts || rework != tc.want {
			t.Errorf("%s: got %d/%d, want %d/%d", tc.name, attempts, rework, tc.wantAttempts, tc.want)
		}
	}
	empty := NewCapsPolicy(fakeTasks{}, &fakeSettings{}, fakeStepCaps{})
	if attempts, rework := empty.Resolve(context.Background(), "task-1", ciCriterion()); attempts != DefaultCap || rework != DefaultCap {
		t.Fatalf("shipped defaults = %d/%d", attempts, rework)
	}
}
