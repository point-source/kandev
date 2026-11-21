package models

import "testing"

func TestTaskCompletionVerdictSourceFollowsTheVerifier(t *testing.T) {
	cases := []struct {
		name      string
		criterion TaskCompletionCriterion
		want      string
	}{
		{"unverified", TaskCompletionCriterion{Kind: TaskCompletionCriterionCheck}, ""},
		{"check by its runner", TaskCompletionCriterion{Kind: TaskCompletionCriterionCheck, VerifierKind: "kandev_ci"}, TaskCompletionVerdictProgram},
		{"review by seats", TaskCompletionCriterion{Kind: TaskCompletionCriterionReview, VerifierKind: "review_seats"}, TaskCompletionVerdictAgent},
		{"review accepted by a person", TaskCompletionCriterion{Kind: TaskCompletionCriterionReview, VerifierKind: TaskCompletionVerifierHuman}, TaskCompletionVerdictPerson},
		{"attested by an agent", TaskCompletionCriterion{Kind: TaskCompletionCriterionAttested, VerifierKind: "agent"}, TaskCompletionVerdictAgent},
		{"attested by a person", TaskCompletionCriterion{Kind: TaskCompletionCriterionAttested, VerifierKind: TaskCompletionVerifierHuman}, TaskCompletionVerdictPerson},
	}
	for _, tc := range cases {
		if got := TaskCompletionVerdictSource(tc.criterion); got != tc.want {
			t.Errorf("%s: verdict source = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestTaskCompletionCriterionIDNamesTheRunner(t *testing.T) {
	if got := TaskCompletionCriterionID(TaskCompletionCriterionCheck, TaskCompletionCheckRunnerGitHub); got != "github_check" {
		t.Fatalf("check id = %q", got)
	}
	if got := TaskCompletionCriterionID(TaskCompletionCriterionReview, ""); got != "review" {
		t.Fatalf("review id = %q", got)
	}
}
