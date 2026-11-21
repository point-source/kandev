package completiongates

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/kandev/kandev/internal/auth/authn"
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/task/models"
)

type fakeAgents struct{ ceo string }

func (f fakeAgents) ActiveAgent(_ context.Context, _, agentID string) bool { return agentID == "owner" }
func (f fakeAgents) WorkspaceCEO(context.Context, string) string           { return f.ceo }

type spyRuns struct{ keys, agents []string }

func (s *spyRuns) QueueRun(_ context.Context, agentID, _, _, key string) error {
	s.agents = append(s.agents, agentID)
	s.keys = append(s.keys, key)
	return nil
}

type fakeHeld struct {
	held []models.HeldTaskCompletionCriterion
}

func (f fakeHeld) ListHeldCompletionCriteria(context.Context, string) ([]models.HeldTaskCompletionCriterion, error) {
	return f.held, nil
}

func newControlService(t *testing.T, criterion models.TaskCompletionCriterion, held ...models.HeldTaskCompletionCriterion) (*Service, *fakeGates) {
	t.Helper()
	gates := newFakeGates()
	gates.setCriteria("task-1", criterion)
	svc := NewService(Dependencies{
		Gates: gates, PRs: newFakePRs(), Tasks: fakeTasks{assignee: "worker"}, Reviews: &fakeReviews{},
		Settings: &fakeSettings{}, Agents: fakeAgents{ceo: "ceo"}, Runs: &spyRuns{}, Held: fakeHeld{held: held},
		Logger: newTestLogger(t),
	})
	return svc, gates
}

func ownedCriterion(kind, id string) models.TaskCompletionCriterion {
	criterion := ciCriterion()
	criterion.OwnerKind, criterion.OwnerID = kind, id
	criterion.HoldKind = HoldReworkLimitReached
	return criterion
}

// @covers AC-OFFICE-COMPLETION-GATES-005.3, AC-OFFICE-COMPLETION-GATES-007.5
func TestExtendAsAgentRequiresOwnerOrCEOWithinTheCeiling(t *testing.T) {
	svc, gates := newControlService(t, ownedCriterion(ownerKindAgent, "owner"))
	request := ExtendRequest{ReworkLimit: 5}
	if _, err := svc.ExtendAsAgent(context.Background(), "task-1", "ci", "worker", request); !errors.Is(err, ErrNotCriterionOwner) {
		t.Fatalf("assignee err = %v", err)
	}
	if _, err := svc.ExtendAsAgent(context.Background(), "task-1", "ci", "stranger", request); !errors.Is(err, ErrNotCriterionOwner) {
		t.Fatalf("stranger err = %v", err)
	}
	if _, err := svc.ExtendAsAgent(context.Background(), "task-1", "ci", "owner", ExtendRequest{ReworkLimit: 50}); !errors.Is(err, ErrLimitAboveCeiling) {
		t.Fatalf("above ceiling err = %v", err)
	}
	for _, agent := range []string{"owner", "ceo"} {
		if _, err := svc.ExtendAsAgent(context.Background(), "task-1", "ci", agent, request); err != nil {
			t.Fatalf("%s extend: %v", agent, err)
		}
	}
	if len(gates.controls) != 2 || gates.controls[0].Action != models.TaskCompletionControlExtendLimits || gates.controls[0].LimitsConfirmedBy != "" {
		t.Fatalf("controls = %+v", gates.controls)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-007.5
func TestExtendAsPersonConfirmsAboveTheCeiling(t *testing.T) {
	svc, gates := newControlService(t, ownedCriterion(ownerKindHuman, "user-1"))
	ctx := authn.WithIdentity(context.Background(), authn.Identity{UserID: "user-1", Role: authn.RoleAdmin, Synthetic: true})
	if _, err := svc.ExtendAsPerson(ctx, "task-1", "ci", ExtendRequest{ExpectedCriterionRevision: 1, ReworkLimit: 50, Reason: "big task"}); err != nil {
		t.Fatalf("extend: %v", err)
	}
	if got := gates.controls[0]; got.LimitsConfirmedBy != "user-1" || got.ActorKind != "human" {
		t.Fatalf("control = %+v", got)
	}
	if _, err := svc.ExtendAsPerson(context.Background(), "task-1", "ci", ExtendRequest{ReworkLimit: 4}); !errors.Is(err, ErrHumanRequired) {
		t.Fatalf("anonymous err = %v", err)
	}
}

func TestSettingsValidation(t *testing.T) {
	svc, _ := newControlService(t, ciCriterion())
	valid := officemodels.CompletionGateSettings{
		DefaultCriteria:          []officemodels.CompletionGateCriterionSpec{{Kind: models.TaskCompletionCriterionCheck, Runner: models.TaskCompletionCheckRunnerGitHub, ProtectedPaths: []string{"tests/**"}}},
		DefaultVerificationLimit: 3, DefaultReworkLimit: 3, VerificationLimitCeiling: 10, ReworkLimitCeiling: 10,
	}
	saved, err := svc.UpdateSettings(context.Background(), "ws-1", valid)
	if err != nil || len(saved.DefaultCriteria) != 1 || saved.ReworkLimitCeiling != 10 {
		t.Fatalf("saved = %+v, %v", saved, err)
	}
	for name, mutate := range map[string]func(*officemodels.CompletionGateSettings){
		"default above ceiling": func(s *officemodels.CompletionGateSettings) { s.DefaultReworkLimit = 11 },
		"zero limit":            func(s *officemodels.CompletionGateSettings) { s.VerificationLimitCeiling = 0 },
		"bad kind":              func(s *officemodels.CompletionGateSettings) { s.DefaultCriteria[0].Kind = "attested" },
		"bad path":              func(s *officemodels.CompletionGateSettings) { s.DefaultCriteria[0].ProtectedPaths = []string{"[x"} },
		"negative review wait":  func(s *officemodels.CompletionGateSettings) { s.DefaultReviewWaitLimit = -1 },
		"check without runner":  func(s *officemodels.CompletionGateSettings) { s.DefaultCriteria[0].Runner = "" },
	} {
		candidate := valid
		candidate.DefaultCriteria = append([]officemodels.CompletionGateCriterionSpec(nil), valid.DefaultCriteria...)
		mutate(&candidate)
		if _, err := svc.UpdateSettings(context.Background(), "ws-1", candidate); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-003.2, AC-OFFICE-COMPLETION-GATES-007.4
func TestInboxShowsProtectedHoldsAndPersonOwnedOrUnescalatedCapHolds(t *testing.T) {
	now := time.Now()
	protected := ownedCriterion(ownerKindAgent, "owner")
	protected.HoldKind, protected.HeldAt = HoldProtectedFilesChanged, &now
	escalated := ownedCriterion(ownerKindAgent, "owner")
	escalated.EscalatedAt = &now
	svc, _ := newControlService(t, ciCriterion(),
		models.HeldTaskCompletionCriterion{TaskID: "t1", TaskTitle: "A", Criterion: protected},
		models.HeldTaskCompletionCriterion{TaskID: "t2", TaskTitle: "B", Criterion: escalated},
		models.HeldTaskCompletionCriterion{TaskID: "t3", TaskTitle: "C", Criterion: ownedCriterion(ownerKindHuman, "user-1")},
		models.HeldTaskCompletionCriterion{TaskID: "t4", TaskTitle: "D", Criterion: ownedCriterion(ownerKindAgent, "owner")},
	)
	items, err := svc.InboxItems(context.Background(), "ws-1")
	if err != nil || len(items) != 3 || items[0].Type != InboxTypeHold ||
		items[1].EntityID != "t3" || items[2].EntityID != "t4" || items[2].Type != InboxTypeHold {
		t.Fatalf("items = %+v, %v", items, err)
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-007.3
func TestAssigneeRunsAreHeld(t *testing.T) {
	svc, gates := newControlService(t, ownedCriterion(ownerKindAgent, "owner"))
	if !svc.AssigneeRunHeld(context.Background(), "task-1", "worker") || svc.AssigneeRunHeld(context.Background(), "task-1", "reviewer") {
		t.Fatal("hold should stop only the assignee")
	}
	gates.setCriteria("task-1", ciCriterion())
	if svc.AssigneeRunHeld(context.Background(), "task-1", "worker") {
		t.Fatal("unheld criterion stopped the assignee")
	}
}

// @covers AC-OFFICE-COMPLETION-GATES-007.4
func TestEscalationWakesTheOwnerOncePerHold(t *testing.T) {
	runs := &spyRuns{}
	escalator := NewEscalator(newFakeGates(), fakeTasks{assignee: "worker"}, fakeAgents{ceo: "ceo"}, runs, nil, newTestLogger(t))
	heldAt := time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
	owned := ownedCriterion(ownerKindAgent, "owner")
	owned.HeldAt = &heldAt
	escalator.EscalateHold(context.Background(), "task-1", owned)
	gone := ownedCriterion(ownerKindAgent, "departed")
	gone.HeldAt = &heldAt
	escalator.EscalateHold(context.Background(), "task-1", gone)
	escalator.EscalateHold(context.Background(), "task-1", ownedCriterion(ownerKindHuman, "user-1"))
	if len(runs.agents) != 2 || runs.agents[0] != "owner" || runs.agents[1] != "ceo" {
		t.Fatalf("woken = %v", runs.agents)
	}
	if runs.keys[0] != "completion_gate_held:task-1:ci:2026-11-20T12:00:00Z" {
		t.Fatalf("key = %q", runs.keys[0])
	}
}
