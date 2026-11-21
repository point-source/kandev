package completiongates

import (
	"context"
	"errors"
	"testing"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
	taskservice "github.com/kandev/kandev/internal/task/service"
)

type fakeStages struct{ review bool }

func (f fakeStages) HasReviewStage(context.Context, string) (bool, error) { return f.review, nil }

func newCreateService(t *testing.T, defaults []officemodels.CompletionGateCriterionSpec, review bool) *Service {
	t.Helper()
	settings := &fakeSettings{settings: officemodels.CompletionGateSettings{DefaultCriteria: defaults}}
	return NewService(Dependencies{Gates: newFakeGates(), PRs: newFakePRs(), Tasks: fakeTasks{}, Reviews: &fakeReviews{},
		Settings: settings, Stages: fakeStages{review: review}, Logger: newTestLogger(t)})
}

func resolution(creator string, requested ...models.TaskCompletionCriterionRequest) models.CompletionCriteriaResolution {
	input := models.CompletionCriteriaResolution{WorkspaceID: "ws-1", WorkflowID: "wf", IsOffice: true, HasRepository: true,
		Creator: models.CompletionCriteriaCreator{Kind: creator, ID: creator + "-1"}}
	if requested != nil {
		input.Requested = &requested
	}
	return input
}

func kinds(criteria []models.TaskCompletionCriterion) []string {
	out := make([]string, len(criteria))
	for index, criterion := range criteria {
		out[index] = criterion.Kind
	}
	return out
}

var ciDefault = []officemodels.CompletionGateCriterionSpec{{Kind: models.TaskCompletionCriterionCheck, Runner: models.TaskCompletionCheckRunnerGitHub, ProtectedPaths: []string{"tests/**"}}}

// @covers AC-OFFICE-COMPLETION-GATES-001.1, AC-OFFICE-COMPLETION-GATES-001.2
func TestResolveCreateCriteriaAppliesDefaults(t *testing.T) {
	svc := newCreateService(t, ciDefault, true)
	criteria, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorAgent))
	if err != nil || len(criteria) != 1 || criteria[0].OwnerKind != "agent" || criteria[0].OwnerID != "agent-1" {
		t.Fatalf("default criteria = %+v, %v", criteria, err)
	}
	review := models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionReview}
	criteria, err = svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorAgent, review))
	if err != nil || len(criteria) != 2 {
		t.Fatalf("agent cannot drop defaults: %v, %v", kinds(criteria), err)
	}
	criteria, err = svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorHuman, review))
	if err != nil || len(criteria) != 1 || criteria[0].Kind != models.TaskCompletionCriterionReview {
		t.Fatalf("person can replace defaults: %v, %v", kinds(criteria), err)
	}
	empty := []models.TaskCompletionCriterionRequest{}
	input := resolution(models.CompletionCriteriaCreatorHuman)
	input.Requested = &empty
	if criteria, err := svc.ResolveCreateCriteria(context.Background(), input); err != nil || len(criteria) != 0 {
		t.Fatalf("person removing all defaults = %v, %v", kinds(criteria), err)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-001.3, AC-OFFICE-COMPLETION-GATES-001.5, AC-OFFICE-COMPLETION-GATES-007.2
func TestResolveCreateCriteriaRefusals(t *testing.T) {
	svc := newCreateService(t, nil, false)
	noRepo := resolution(models.CompletionCriteriaCreatorAgent, models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionCheck, Runner: models.TaskCompletionCheckRunnerGitHub})
	noRepo.HasRepository = false
	if _, err := svc.ResolveCreateCriteria(context.Background(), noRepo); !errors.Is(err, ErrCriteriaRequireRepository) {
		t.Fatalf("no repository err = %v", err)
	}
	if _, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorAgent,
		models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionReview})); !errors.Is(err, ErrCriteriaRequireReviewStage) {
		t.Fatalf("no review stage err = %v", err)
	}
	if _, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorAgent,
		models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionCheck, Runner: models.TaskCompletionCheckRunnerGitHub, ProtectedPaths: []string{" "}})); refusalCode(err) != "invalid_protected_path" {
		t.Fatalf("empty path err = %v", err)
	}
	above := models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionCheck, Runner: models.TaskCompletionCheckRunnerGitHub, ReworkLimit: 99}
	if _, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorAgent, above)); refusalCode(err) != "completion_limit_above_ceiling" {
		t.Fatalf("agent above ceiling err = %v", err)
	}
	criteria, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorHuman, above))
	if err != nil || criteria[0].Config.LimitsConfirmedBy != "human-1" {
		t.Fatalf("person above ceiling = %+v, %v", criteria, err)
	}
}

func TestInapplicableDefaultsAreSkipped(t *testing.T) {
	svc := newCreateService(t, append(append([]officemodels.CompletionGateCriterionSpec(nil), ciDefault...),
		officemodels.CompletionGateCriterionSpec{Kind: models.TaskCompletionCriterionReview}), false)
	criteria, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorAgent))
	if err != nil || len(criteria) != 1 || criteria[0].Kind != models.TaskCompletionCriterionCheck {
		t.Fatalf("criteria = %v, %v", kinds(criteria), err)
	}
	noRepo := resolution(models.CompletionCriteriaCreatorAgent)
	noRepo.HasRepository = false
	if criteria, err := svc.ResolveCreateCriteria(context.Background(), noRepo); err != nil || len(criteria) != 0 {
		t.Fatalf("no-repository defaults = %v, %v", kinds(criteria), err)
	}
}

func refusalCode(err error) string {
	code, _ := taskservice.IsCompletionCriteriaRefusal(err)
	return code
}

func TestDuplicateRequestedKindsAreRefused(t *testing.T) {
	svc := newCreateService(t, nil, true)
	ci := models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionCheck, Runner: models.TaskCompletionCheckRunnerGitHub}
	_, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorHuman, ci, ci))
	if refusalCode(err) != "invalid_completion_criterion" {
		t.Fatalf("duplicate kinds err = %v", err)
	}
}
