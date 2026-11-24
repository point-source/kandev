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
	prs      PRSource
	tasks    TaskReader
	runs     RunQueue
	settings SettingsStore
	limits   *LimitsPolicy
	agents   AgentDirectory
	held     HeldLister
	stages   WorkflowStageReader
	seater   ReviewerSeater
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
	Steps    StepLimitsReader
	Agents   AgentDirectory
	Runs     RunQueue
	History  HistoryReader
	Held     HeldLister
	Stages   WorkflowStageReader
	Ledger   TransitionReader
	Bus      bus.EventBus
	Logger   *logger.Logger
}

// NewService builds the Office completion-gate service. Routing stays
// inactive until SetNavigator is called.
func NewService(deps Dependencies) *Service {
	review := NewReviewVerifier(deps.Gates, deps.PRs, deps.Tasks, deps.Reviews, deps.Logger)
	router := NewRouter(deps.Gates, deps.PRs, nil, review, deps.Logger)
	limits := NewLimitsPolicy(deps.Tasks, deps.Settings, deps.Steps)
	router.SetLimitResolver(limits.Resolve)
	var escalator *Escalator
	if deps.Agents != nil && deps.Runs != nil {
		escalator = NewEscalator(deps.Gates, deps.Tasks, deps.Agents, deps.Runs, deps.History, deps.Logger)
		router.SetEscalator(escalator)
	}
	ci := NewCIVerifier(deps.Gates, deps.PRs, deps.Bus, deps.Logger)
	ci.SetPolicy(router)
	review.SetPolicy(router)
	return &Service{
		gates: deps.Gates, prs: deps.PRs, tasks: deps.Tasks, runs: deps.Runs, settings: deps.Settings, limits: limits, agents: deps.Agents,
		held: deps.Held, stages: deps.Stages, bus: deps.Bus, CI: ci, Review: review, Router: router,
		escalator: escalator, ledger: deps.Ledger, transitions: newTransitionTracker(), logger: deps.Logger,
	}
}

// SetReviewerSeater installs the seater the reviewer sweep uses to clear a
// no-reviewer hold once a reviewer agent can approve.
func (s *Service) SetReviewerSeater(seater ReviewerSeater) { s.seater = seater }

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
		events.GitHubTaskPRDeleted: s.async(ctx, s.handleHeadsChanged),
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
		s.sweepReviewWaits(ctx, time.Now())
		s.sweepReviewerSeats(ctx)
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
		s.Router.EvaluateApproval(ctx, taskID)
	}
}

// PersonAction is a person's decision on one criterion at the heads they saw.
type PersonAction struct {
	ExpectedCriterionRevision int64  `json:"expected_criterion_revision"`
	ExpectedSubjectRevision   string `json:"expected_subject_revision"`
	Reason                    string `json:"reason"`
}

// Allow accepts a check's protected-file changes at the heads shown.
func (s *Service) Allow(ctx context.Context, taskID, criterionID string, request PersonAction) (*models.TaskCompletionGateSnapshot, error) {
	outcome, err := s.applyPersonControl(ctx, taskID, criterionID, models.TaskCompletionControlAllowProtected, request)
	if err != nil {
		return nil, err
	}
	s.CI.Schedule(context.WithoutCancel(ctx), taskID)
	return outcome.Snapshot, nil
}

// SendBack fails a criterion at the heads shown with the person's reason and
// routes like a seat rejection: it counts a rework round, returns the task to
// the work stage, and wakes the worker there.
func (s *Service) SendBack(ctx context.Context, taskID, criterionID string, request PersonAction) (*models.TaskCompletionGateSnapshot, error) {
	if strings.TrimSpace(request.Reason) == "" {
		return nil, ErrReasonRequired
	}
	outcome, err := s.applyPersonControl(ctx, taskID, criterionID, models.TaskCompletionControlSendBack, request)
	if err != nil {
		return nil, err
	}
	background := context.WithoutCancel(ctx)
	s.routeSendBack(background, taskID, findCriterionByID(outcome.Snapshot, criterionID), strings.TrimSpace(request.Reason))
	s.Router.EvaluateApproval(background, taskID)
	return s.gates.GetTaskCompletionGate(ctx, taskID)
}

// routeSendBack counts a person's send back once per heads and returns the
// task to work, waking the worker when it is already there. A send back held
// at the rework limit routes the same way once the limit is raised.
func (s *Service) routeSendBack(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion, reason string) {
	key := "send-back:" + criterion.ResultSubjectRevision
	if s.Router.sendBack(ctx, taskID, criterion, key) {
		s.wakeAssignee(ctx, taskID, reason, key)
	}
}

// Accept verifies a held review at the heads shown as the person's verdict.
// It never moves the task; normal completion follows once every criterion
// passes.
func (s *Service) Accept(ctx context.Context, taskID, criterionID string, request PersonAction) (*models.TaskCompletionGateSnapshot, error) {
	if strings.TrimSpace(request.Reason) == "" {
		return nil, ErrReasonRequired
	}
	outcome, err := s.applyPersonControl(ctx, taskID, criterionID, models.TaskCompletionControlAccept, request)
	if err != nil {
		return nil, err
	}
	background := context.WithoutCancel(ctx)
	s.Router.EvaluateApproval(background, taskID)
	if outcome.Snapshot != nil && !outcome.Snapshot.Blocked {
		s.Router.retryCompletion(background, taskID, request.ExpectedSubjectRevision)
	}
	return s.gates.GetTaskCompletionGate(ctx, taskID)
}

// Approve passes an approval that asks the person, at the heads shown, as the
// person's verdict, then completes the task through its normal transition
// once every criterion passes. The ask is re-evaluated first, so an approval
// whose other criteria stopped passing is refused as changed.
func (s *Service) Approve(ctx context.Context, taskID, criterionID string, request PersonAction) (*models.TaskCompletionGateSnapshot, error) {
	if _, err := s.authorizeHuman(ctx, taskID); err != nil {
		return nil, err
	}
	unlock := s.Router.approvals.lock(taskID)
	s.Router.evaluateApprovalLocked(ctx, taskID)
	outcome, err := s.applyPersonControl(ctx, taskID, criterionID, models.TaskCompletionControlApprove, request)
	unlock()
	if err != nil {
		return nil, err
	}
	if outcome.Snapshot != nil && !outcome.Snapshot.Blocked {
		s.Router.retryCompletion(context.WithoutCancel(ctx), taskID, request.ExpectedSubjectRevision)
	}
	return outcome.Snapshot, nil
}

func (s *Service) applyPersonControl(ctx context.Context, taskID, criterionID, action string, request PersonAction) (*models.TaskCompletionControlOutcome, error) {
	actorID, err := s.authorizeHuman(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return s.gates.ApplyTaskCompletionControl(ctx, models.TaskCompletionControlChange{
		TaskID: taskID, CriterionID: criterionID, CriterionRevision: request.ExpectedCriterionRevision,
		SubjectRevision: request.ExpectedSubjectRevision, Action: action, ActorKind: "human", ActorID: actorID,
		Reason: strings.TrimSpace(request.Reason),
	})
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
