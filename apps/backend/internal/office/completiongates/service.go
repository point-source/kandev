package completiongates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/auth/authn"
	"github.com/kandev/kandev/internal/authz"
	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
)

// Errors returned to human callers.
var (
	ErrHumanRequired      = errors.New("a signed-in person is required")
	ErrUnsupportedControl = errors.New("completion control is not supported")
	ErrReasonRequired     = errors.New("a reason is required")
)

// Service owns the Office completion-gate verifiers, routing and controls.
type Service struct {
	gates    GateService
	tasks    TaskReader
	runs     RunQueue
	settings SettingsStore
	caps     *CapsPolicy
	agents   AgentDirectory
	held     HeldLister
	stages   ReviewStageReader
	bus      bus.EventBus
	CI       *CIVerifier
	Review   *ReviewVerifier
	Router   *Router
	logger   *logger.Logger

	escalator   *Escalator
	ledger      TransitionReader
	transitions *transitionTracker

	mu     sync.Mutex
	cancel context.CancelFunc
	subs   []bus.Subscription
	wg     sync.WaitGroup
}

// Dependencies are the collaborators of the Office completion-gate service.
type Dependencies struct {
	Gates    GateService
	PRs      PRSource
	Tasks    TaskReader
	Reviews  ReviewStore
	Settings SettingsStore
	Steps    StepCapsReader
	Agents   AgentDirectory
	Runs     RunQueue
	History  HistoryReader
	Held     HeldLister
	Stages   ReviewStageReader
	Ledger   TransitionReader
	Bus      bus.EventBus
	Logger   *logger.Logger
}

// NewService builds the Office completion-gate service. Routing stays
// inactive until SetNavigator is called.
func NewService(deps Dependencies) *Service {
	review := NewReviewVerifier(deps.Gates, deps.PRs, deps.Tasks, deps.Reviews, deps.Logger)
	router := NewRouter(deps.Gates, deps.PRs, nil, review, deps.Logger)
	caps := NewCapsPolicy(deps.Tasks, deps.Settings, deps.Steps)
	router.SetCapResolver(caps.Resolve)
	var escalator *Escalator
	if deps.Agents != nil && deps.Runs != nil {
		escalator = NewEscalator(deps.Gates, deps.Tasks, deps.Agents, deps.Runs, deps.History, deps.Logger)
		router.SetEscalator(escalator)
	}
	ci := NewCIVerifier(deps.Gates, deps.PRs, deps.Bus, deps.Logger)
	ci.SetPolicy(router)
	review.SetPolicy(router)
	return &Service{
		gates: deps.Gates, tasks: deps.Tasks, runs: deps.Runs, settings: deps.Settings, caps: caps, agents: deps.Agents,
		held: deps.Held, stages: deps.Stages, bus: deps.Bus, CI: ci, Review: review, Router: router,
		escalator: escalator, ledger: deps.Ledger, transitions: newTransitionTracker(), logger: deps.Logger,
	}
}

// SetNavigator activates routing with the workflow navigator.
func (s *Service) SetNavigator(navigator Navigator) { s.Router.SetNavigator(navigator) }

// Start starts the verifiers and the routing subscriptions.
func (s *Service) Start(parent context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	if err := s.CI.Start(ctx); err != nil {
		cancel()
		return err
	}
	s.cancel = cancel
	if s.escalator != nil {
		s.wg.Add(1)
		go s.escalationLoop(ctx)
	}
	if s.bus == nil {
		return nil
	}
	for subject, handler := range map[string]bus.EventHandler{
		events.TaskMoved:           s.async(ctx, s.handleTaskMoved),
		events.TaskUpdated:         s.async(ctx, s.handleTaskUpdated),
		events.GitHubTaskPRUpdated: s.async(ctx, s.handleHeadsChanged),
	} {
		sub, err := s.bus.Subscribe(subject, handler)
		if err != nil {
			return fmt.Errorf("subscribe to %s: %w", subject, err)
		}
		s.subs = append(s.subs, sub)
	}
	return nil
}

// Stop stops routing and the verifiers and waits for running work.
func (s *Service) Stop() {
	s.mu.Lock()
	cancel, subs := s.cancel, s.subs
	s.cancel, s.subs = nil, nil
	s.mu.Unlock()
	if cancel == nil {
		return
	}
	for _, sub := range subs {
		_ = sub.Unsubscribe()
	}
	cancel()
	s.wg.Wait()
	s.CI.Stop()
}

// escalationLoop retries hold escalations that did not reach an agent.
func (s *Service) escalationLoop(ctx context.Context) {
	defer s.wg.Done()
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		if err := s.escalator.sweep(ctx); err != nil && ctx.Err() == nil {
			s.logger.Warn("completion gate escalation sweep failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Service) async(ctx context.Context, handle func(context.Context, map[string]any)) bus.EventHandler {
	return func(_ context.Context, event *bus.Event) error {
		payload := payloadMap(event.Data)
		if payload == nil || ctx.Err() != nil {
			return nil
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			handle(ctx, payload)
		}()
		return nil
	}
}

func (s *Service) handleHeadsChanged(ctx context.Context, payload map[string]any) {
	if taskID, _ := payload["task_id"].(string); taskID != "" {
		s.Router.OnHeadsChanged(ctx, taskID)
	}
}

// ProtectedPathsAction is a person's decision on a protected-path hold.
type ProtectedPathsAction struct {
	Action                    string `json:"action"`
	ExpectedCriterionRevision int64  `json:"expected_criterion_revision"`
	ExpectedSubjectRevision   string `json:"expected_subject_revision"`
	Reason                    string `json:"reason"`
}

// Protected-path actions.
const (
	ProtectedPathsAllow    = "allow"
	ProtectedPathsSendBack = "send_back"
)

// DecideProtectedPaths applies a person's Allow or Send back decision.
func (s *Service) DecideProtectedPaths(ctx context.Context, taskID, criterionID string, request ProtectedPathsAction) (*models.TaskCompletionGateSnapshot, error) {
	actorID, err := s.authorizeHuman(ctx, taskID)
	if err != nil {
		return nil, err
	}
	control := models.TaskCompletionControlChange{
		TaskID: taskID, CriterionID: criterionID, CriterionRevision: request.ExpectedCriterionRevision,
		SubjectRevision: request.ExpectedSubjectRevision, ActorKind: "human", ActorID: actorID,
		Reason: strings.TrimSpace(request.Reason),
	}
	switch request.Action {
	case ProtectedPathsAllow:
		control.Action = models.TaskCompletionControlAllowProtected
	case ProtectedPathsSendBack:
		if control.Reason == "" {
			return nil, ErrReasonRequired
		}
		control.Action = models.TaskCompletionControlSendBackProtected
	default:
		return nil, ErrUnsupportedControl
	}
	outcome, err := s.gates.ApplyTaskCompletionControl(ctx, control)
	if err != nil {
		return nil, err
	}
	snapshot := outcome.Snapshot
	background := context.WithoutCancel(ctx)
	if request.Action == ProtectedPathsSendBack {
		criterion := findCriterionByID(snapshot, criterionID)
		key := "send-back:" + request.ExpectedSubjectRevision
		if s.Router.sendBack(background, taskID, criterion, key) {
			s.wakeAssignee(background, taskID, control.Reason, key)
		}
		return s.gates.GetTaskCompletionGate(ctx, taskID)
	}
	s.CI.Schedule(background, taskID)
	return snapshot, nil
}

// wakeAssignee queues the assignee's rework run carrying a person's
// send-back reason.
func (s *Service) wakeAssignee(ctx context.Context, taskID, reason, key string) {
	if s.runs == nil {
		return
	}
	task, err := s.tasks.GetTask(ctx, taskID)
	if err != nil || task == nil || task.AssigneeAgentProfileID == "" {
		return
	}
	payload, _ := json.Marshal(map[string]string{"task_id": taskID, "decision_comment": reason})
	if err := s.runs.QueueRun(ctx, task.AssigneeAgentProfileID, RunReasonChangesRequested, string(payload),
		"completion_gate_send_back:"+taskID+":"+key); err != nil {
		s.logger.Warn("completion gate send-back wake failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

// authorizeHuman requires a person with task write access and returns the
// actor ID recorded in the gate history.
func (s *Service) authorizeHuman(ctx context.Context, taskID string) (string, error) {
	identity, ok := authn.IdentityFromContext(ctx)
	if !ok || strings.TrimSpace(identity.UserID) == "" {
		return "", ErrHumanRequired
	}
	if _, isRun := models.CompletionGateCallerFromContext(ctx); isRun {
		return "", ErrHumanRequired
	}
	if err := s.gates.AuthorizeTaskScope(ctx, taskID, authz.ScopeTaskWrite); err != nil {
		return "", err
	}
	return identity.UserID, nil
}

func transitionKey(value any) string {
	if number, ok := value.(float64); ok {
		return strconv.FormatFloat(number, 'f', -1, 64)
	}
	return fmt.Sprint(value)
}
