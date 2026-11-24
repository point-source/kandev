package completiongates

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

type approvalSpec = officemodels.CompletionGateCriterionSpec

func approvalCriterion() models.TaskCompletionCriterion {
	return models.TaskCompletionCriterion{ID: "approval", Kind: models.TaskCompletionCriterionApproval, CriterionRevision: 1}
}

func askedApproval() models.TaskCompletionCriterion {
	criterion := approvalCriterion()
	criterion.Result, criterion.ResultReason = models.TaskCompletionResultPending, models.TaskCompletionReasonAwaitingPerson
	criterion.ResultSubjectRevision = "pr-1@h1"
	return criterion
}

func newApprovalService(t *testing.T, current string, criteria ...models.TaskCompletionCriterion) (*Service, *fakeGates, *fakeNavigator, *payloadRuns) {
	t.Helper()
	svc, gates, navigator, runs, _ := newApprovalServiceWithPRs(t, current, criteria...)
	return svc, gates, navigator, runs
}

func newApprovalServiceWithPRs(t *testing.T, current string, criteria ...models.TaskCompletionCriterion) (*Service, *fakeGates, *fakeNavigator, *payloadRuns, *fakePRs) {
	t.Helper()
	gates, prs, runs := newFakeGates(), newFakePRs(), &payloadRuns{}
	gates.setCriteria("task-1", criteria...)
	prs.links["task-1"] = []PRLink{{ID: "pr-1", HeadSHA: "h1"}}
	svc := NewService(Dependencies{
		Gates: gates, PRs: prs, Tasks: fakeTasks{assignee: "worker"}, Reviews: &fakeReviews{},
		Settings: &fakeSettings{}, Agents: fakeAgents{ceo: "ceo"}, Runs: runs, Held: fakeHeld{},
		Logger: newTestLogger(t),
	})
	navigator := &fakeNavigator{placement: officePlacement(current)}
	svc.SetNavigator(navigator)
	return svc, gates, navigator, runs, prs
}

func lastApprovalRecord(t *testing.T, gates *fakeGates) models.TaskCompletionEvaluationChange {
	t.Helper()
	records := gates.records()
	for index := len(records) - 1; index >= 0; index-- {
		if records[index].CriterionID == "approval" {
			return records[index]
		}
	}
	t.Fatal("no approval evaluation recorded")
	return models.TaskCompletionEvaluationChange{}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.3
func TestApprovalWaitsForTheWorkStageThenForTheOtherCriteria(t *testing.T) {
	svc, gates, navigator, _ := newApprovalService(t, "work", ciCriterion(), reviewCriterion(), approvalCriterion())
	gates.setBlockers("task-1", "ci", "review", "approval")

	svc.Router.EvaluateApproval(context.Background(), "task-1")
	got := lastApprovalRecord(t, gates)
	if got.Result != models.TaskCompletionResultPending || got.Reason != models.TaskCompletionReasonAwaitingCriteria ||
		!slices.Equal(got.Detail, []string{ApprovalWaitsForWorkStage}) || got.SubjectRevision != "pr-1@h1" {
		t.Fatalf("in work = %+v", got)
	}

	navigator.placement = officePlacement("review")
	svc.Router.EvaluateApproval(context.Background(), "task-1")
	if got := lastApprovalRecord(t, gates); !slices.Equal(got.Detail, []string{"check", "review"}) {
		t.Fatalf("past work = %+v", got)
	}

	gates.setBlockers("task-1", "approval")
	svc.Router.EvaluateApproval(context.Background(), "task-1")
	if got := lastApprovalRecord(t, gates); got.Reason != models.TaskCompletionReasonAwaitingPerson || len(got.Detail) != 0 {
		t.Fatalf("others pass = %+v", got)
	}
	if len(navigator.moves) != 0 {
		t.Fatalf("waiting moved the task: %v", navigator.moves)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.3
func TestApprovalAloneAsksOnceTheTaskLeavesWork(t *testing.T) {
	svc, gates, navigator, _ := newApprovalService(t, "work", approvalCriterion())
	gates.setBlockers("task-1", "approval")
	svc.Router.EvaluateApproval(context.Background(), "task-1")
	if got := lastApprovalRecord(t, gates); got.Reason != models.TaskCompletionReasonAwaitingCriteria {
		t.Fatalf("in work = %+v", got)
	}
	navigator.placement = officePlacement("review")
	svc.Router.EvaluateApproval(context.Background(), "task-1")
	if got := lastApprovalRecord(t, gates); got.Reason != models.TaskCompletionReasonAwaitingPerson {
		t.Fatalf("left work = %+v", got)
	}

	navigator.placement = Placement{WorkflowID: "wf", CurrentStepID: "only"}
	svc.Router.EvaluateApproval(context.Background(), "task-1")
	if got := lastApprovalRecord(t, gates); got.Reason != models.TaskCompletionReasonAwaitingPerson {
		t.Fatalf("no work stage = %+v", got)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.3
func TestApprovalAsksAfterAnotherCriterionPassesAndStaysOffAHeldApproval(t *testing.T) {
	svc, gates, _, _ := newApprovalService(t, "review", ciCriterion(), approvalCriterion())
	gates.setBlockers("task-1", "approval")
	passed := ciCriterion()
	passed.Result, passed.ResultSubjectRevision = models.TaskCompletionResultPass, "pr-1@h1"
	svc.Router.AfterRecord(context.Background(), "task-1", &models.TaskCompletionEvaluationOutcome{
		Criterion: passed, Snapshot: &models.TaskCompletionGateSnapshot{Blocked: true}, Changed: true,
	})
	if got := lastApprovalRecord(t, gates); got.Reason != models.TaskCompletionReasonAwaitingPerson {
		t.Fatalf("after check pass = %+v", got)
	}

	held := approvalCriterion()
	held.HoldKind = HoldReworkLimitReached
	gates.setCriteria("task-1", held)
	before := len(gates.records())
	svc.Router.EvaluateApproval(context.Background(), "task-1")
	if len(gates.records()) != before {
		t.Fatal("a held approval was re-evaluated")
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.4, AC-OFFICE-COMPLETION-GATES-010.5, AC-OFFICE-COMPLETION-GATES-010.6
func TestApproveIsThePersonsVerdictAtTheHeadsShownAndCompletesTheTask(t *testing.T) {
	svc, gates, navigator, _ := newApprovalService(t, "review", askedApproval())
	gates.setBlockers("task-1", "approval")
	request := personAction("")
	if _, err := svc.Approve(context.Background(), "task-1", "approval", request); !errors.Is(err, ErrHumanRequired) {
		t.Fatalf("anonymous err = %v", err)
	}
	run := models.WithCompletionGateCaller(personContext(), models.CompletionGateCaller{AgentProfileID: "worker", RunID: "run-1"})
	if _, err := svc.Approve(run, "task-1", "approval", request); !errors.Is(err, ErrHumanRequired) {
		t.Fatalf("run token err = %v", err)
	}
	gates.setBlockers("task-1")
	if _, err := svc.Approve(personContext(), "task-1", "approval", request); err != nil {
		t.Fatal(err)
	}
	last := gates.controls[len(gates.controls)-1]
	if last.Action != models.TaskCompletionControlApprove || last.SubjectRevision != "pr-1@h1" ||
		last.ActorKind != "human" || last.ActorID != "user-1" {
		t.Fatalf("control = %+v", last)
	}
	if len(navigator.retries) != 1 || len(navigator.moves) != 0 {
		t.Fatalf("retries=%v moves=%v", navigator.retries, navigator.moves)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.4
func TestSendingBackAnApprovalCountsARoundAndReturnsTheTaskToWork(t *testing.T) {
	svc, gates, navigator, _ := newApprovalService(t, "review", askedApproval())
	if _, err := svc.SendBack(personContext(), "task-1", "approval", personAction("redirect still loops")); err != nil {
		t.Fatal(err)
	}
	approval := findCriterionByID(gates.snapshots["task-1"], "approval")
	if approval.ResultReason != models.TaskCompletionReasonSentBack || approval.ReworkCount != 1 {
		t.Fatalf("approval = %+v", approval)
	}
	if len(navigator.moves) != 1 || navigator.moves[0] != "work" {
		t.Fatalf("moves = %v", navigator.moves)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.4
func TestInboxAsksForAnApprovalThatAsksAPerson(t *testing.T) {
	asked := time.Date(2026, 11, 22, 9, 0, 0, 0, time.UTC)
	criterion := askedApproval()
	criterion.ResultAt = &asked
	waiting := approvalCriterion()
	waiting.ResultReason = models.TaskCompletionReasonAwaitingCriteria
	items := inboxItems([]models.HeldTaskCompletionCriterion{
		{TaskID: "t1", TaskTitle: "A", Criterion: criterion},
		{TaskID: "t2", TaskTitle: "B", Criterion: waiting},
	})
	if len(items) != 1 || items[0].Type != InboxTypeApproval || items[0].EntityID != "t1" || !items[0].CreatedAt.Equal(asked) {
		t.Fatalf("items = %+v", items)
	}
	if items[0].Payload["subject_revision"] != "pr-1@h1" || items[0].Payload["asked_at"] != asked.Format(time.RFC3339Nano) {
		t.Fatalf("payload = %+v", items[0].Payload)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.1
func TestApprovalSpecCarriesOnlyAReworkLimit(t *testing.T) {
	approval := func(mutate func(*approvalSpec)) approvalSpec {
		spec := approvalSpec{Kind: models.TaskCompletionCriterionApproval, ReworkLimit: 2}
		if mutate != nil {
			mutate(&spec)
		}
		return spec
	}
	if err := ValidateCriterionSpec(approval(nil)); err != nil {
		t.Fatalf("valid approval: %v", err)
	}
	for name, mutate := range map[string]func(*approvalSpec){
		"runner":             func(s *approvalSpec) { s.Runner = "github" },
		"protected paths":    func(s *approvalSpec) { s.ProtectedPaths = []string{"tests/**"} },
		"verification limit": func(s *approvalSpec) { s.VerificationLimit = 2 },
		"review wait limit":  func(s *approvalSpec) { s.ReviewWaitLimit = 5 },
	} {
		if err := ValidateCriterionSpec(approval(mutate)); err == nil {
			t.Errorf("approval with %s accepted", name)
		}
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.1
func TestApprovalIsCreatedWithoutAReviewStageButNeedsARepository(t *testing.T) {
	svc := newCreateService(t, nil, false)
	request := models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionApproval, ReworkLimit: 2}
	criteria, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorHuman, request))
	if err != nil || len(criteria) != 1 || criteria[0].ID != "approval" || criteria[0].Config.ReworkLimit != 2 {
		t.Fatalf("criteria = %+v, %v", criteria, err)
	}
	input := resolution(models.CompletionCriteriaCreatorHuman, request)
	input.HasRepository = false
	if _, err := svc.ResolveCreateCriteria(context.Background(), input); !errors.Is(err, ErrCriteriaRequireRepository) {
		t.Fatalf("without repository err = %v", err)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.1
func TestApprovalNeedsAStepBetweenWorkAndCompletion(t *testing.T) {
	settings := &fakeSettings{}
	svc := NewService(Dependencies{Gates: newFakeGates(), PRs: newFakePRs(), Tasks: fakeTasks{}, Reviews: &fakeReviews{},
		Settings: settings, Stages: fakeStages{review: true, noStepAfterWork: true}, Logger: newTestLogger(t)})
	request := models.TaskCompletionCriterionRequest{Kind: models.TaskCompletionCriterionApproval}
	_, err := svc.ResolveCreateCriteria(context.Background(), resolution(models.CompletionCriteriaCreatorHuman, request))
	if refusalCode(err) != "completion_criteria_requires_approval_stage" {
		t.Fatalf("create err = %v", err)
	}
	approval := []models.TaskCompletionCriterion{{ID: "approval", Kind: models.TaskCompletionCriterionApproval}}
	err = svc.ValidateCriteriaChange(context.Background(), &models.Task{ID: "task-1", WorkflowID: "wf"}, approval)
	if refusalCode(err) != "completion_criteria_requires_approval_stage" {
		t.Fatalf("edit err = %v", err)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.3
func TestApprovalWaitsForAPullRequestAndWithdrawsTheAskOnDetach(t *testing.T) {
	svc, gates, _, _, prs := newApprovalServiceWithPRs(t, "review", askedApproval())
	gates.setBlockers("task-1", "approval")
	prs.links["task-1"] = nil
	svc.handleHeadsChanged(context.Background(), map[string]any{"task_id": "task-1"})
	got := lastApprovalRecord(t, gates)
	if got.Reason != models.TaskCompletionReasonAwaitingCriteria || got.SubjectRevision != "" ||
		!slices.Equal(got.Detail, []string{ApprovalWaitsForPullRequest}) {
		t.Fatalf("after detach = %+v", got)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-010.3
func TestAnOfficeTaskUpdateWithoutAMoveEvaluatesTheApproval(t *testing.T) {
	svc, gates, _, _ := newApprovalService(t, "review", approvalCriterion())
	gates.setBlockers("task-1", "approval")
	svc.handleTaskUpdated(context.Background(), map[string]any{"task_id": "task-1", "is_from_office": false})
	if len(gates.records()) != 0 {
		t.Fatal("a non-Office update evaluated the approval")
	}
	svc.handleTaskUpdated(context.Background(), map[string]any{"task_id": "task-1", "is_from_office": true})
	if got := lastApprovalRecord(t, gates); got.Reason != models.TaskCompletionReasonAwaitingPerson {
		t.Fatalf("after criteria update = %+v", got)
	}
}

func TestTaskLocksSerializePerTaskAndForgetIdleTasks(t *testing.T) {
	var locks taskLocks
	unlock := locks.lock("task-1")
	acquired := make(chan struct{})
	go func() {
		release := locks.lock("task-1")
		close(acquired)
		release()
	}()
	otherDone := make(chan struct{})
	go func() {
		locks.lock("task-2")()
		close(otherDone)
	}()
	<-otherDone
	select {
	case <-acquired:
		t.Fatal("a second lock on the same task did not wait")
	case <-time.After(20 * time.Millisecond):
	}
	unlock()
	<-acquired
	locks.mu.Lock()
	defer locks.mu.Unlock()
	if len(locks.locks) != 0 {
		t.Fatalf("idle task locks kept: %d", len(locks.locks))
	}
}
