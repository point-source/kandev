package completiongates

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events"
	"github.com/kandev/kandev/internal/events/bus"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/task/repository/repoerrors"
)

const (
	sweepInterval  = time.Minute
	sweepBatchSize = 100
)

// CIVerifier records ci_at_head results from PR checks at each linked head.
type CIVerifier struct {
	gates  GateService
	prs    PRSource
	bus    bus.EventBus
	logger *logger.Logger
	now    func() time.Time

	policyMu sync.RWMutex
	policy   EvaluationPolicy

	mu        sync.Mutex
	cancel    context.CancelFunc
	sub       bus.Subscription
	scheduler *taskScheduler
	loopDone  chan struct{}
}

// NewCIVerifier creates a verifier. Start subscribes it to PR updates.
func NewCIVerifier(gates GateService, prs PRSource, eventBus bus.EventBus, log *logger.Logger) *CIVerifier {
	return &CIVerifier{gates: gates, prs: prs, bus: eventBus, logger: log, now: func() time.Time { return time.Now().UTC() }}
}

// SetPolicy installs the routing policy consulted around each record.
func (v *CIVerifier) SetPolicy(policy EvaluationPolicy) {
	v.policyMu.Lock()
	defer v.policyMu.Unlock()
	v.policy = policy
}

func (v *CIVerifier) currentPolicy() EvaluationPolicy {
	v.policyMu.RLock()
	defer v.policyMu.RUnlock()
	return v.policy
}

// Start subscribes to PR updates, evaluates every unmet CI criterion once,
// and starts the missing-check sweep. It is idempotent and restartable.
func (v *CIVerifier) Start(parent context.Context) error {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.cancel != nil {
		return nil
	}
	ctx, cancel := context.WithCancel(parent)
	v.cancel = cancel
	v.scheduler = newTaskScheduler(v.evaluateLogged)
	if v.bus != nil {
		sub, err := v.bus.Subscribe(events.GitHubTaskPRUpdated, v.handleTaskPRUpdated(ctx))
		if err != nil {
			cancel()
			v.cancel = nil
			return fmt.Errorf("subscribe to PR updates: %w", err)
		}
		v.sub = sub
	}
	v.loopDone = make(chan struct{})
	go v.sweepLoop(ctx, v.loopDone)
	return nil
}

// Stop unsubscribes, stops the sweep, and waits for running evaluations.
func (v *CIVerifier) Stop() {
	v.mu.Lock()
	cancel, sub, scheduler, done := v.cancel, v.sub, v.scheduler, v.loopDone
	v.cancel, v.sub, v.scheduler, v.loopDone = nil, nil, nil, nil
	v.mu.Unlock()
	if cancel == nil {
		return
	}
	if sub != nil {
		_ = sub.Unsubscribe()
	}
	cancel()
	<-done
	scheduler.wait()
}

// Schedule requests an evaluation of one task.
func (v *CIVerifier) Schedule(ctx context.Context, taskID string) {
	v.mu.Lock()
	scheduler := v.scheduler
	v.mu.Unlock()
	if scheduler != nil {
		scheduler.schedule(ctx, taskID)
	}
}

func (v *CIVerifier) handleTaskPRUpdated(ctx context.Context) bus.EventHandler {
	return func(_ context.Context, event *bus.Event) error {
		if taskID := taskIDFromPayload(event.Data); taskID != "" {
			v.Schedule(ctx, taskID)
		}
		return nil
	}
}

func taskIDFromPayload(data any) string {
	encoded, err := json.Marshal(data)
	if err != nil {
		return ""
	}
	var payload struct {
		TaskID string `json:"task_id"`
	}
	if json.Unmarshal(encoded, &payload) != nil {
		return ""
	}
	return payload.TaskID
}

func (v *CIVerifier) sweepLoop(ctx context.Context, done chan struct{}) {
	defer close(done)
	v.sweep(ctx, true)
	ticker := time.NewTicker(sweepInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			v.sweep(ctx, false)
		}
	}
}

// sweep schedules unmet CI criteria. At startup it schedules all of them;
// afterwards only never-evaluated ones and those whose missing-check grace
// has expired.
func (v *CIVerifier) sweep(ctx context.Context, all bool) {
	query := models.TaskCompletionSweepQuery{Kind: models.TaskCompletionCriterionCIAtHead, Limit: sweepBatchSize}
	if !all {
		dueBefore := v.now().Add(-ChecksMissingGrace)
		query.DueBefore, query.AwaitingReason = &dueBefore, ReasonAwaitingChecks
	}
	err := forEachSweepPage(ctx, v.gates, query, func(taskID string) { v.Schedule(ctx, taskID) })
	if err != nil && ctx.Err() == nil {
		v.logger.Warn("completion gate CI sweep failed", zap.Error(err))
	}
}

// forEachSweepPage visits every task the query selects, page by page.
func forEachSweepPage(ctx context.Context, gates GateService, query models.TaskCompletionSweepQuery, visit func(string)) error {
	for ctx.Err() == nil {
		ids, err := gates.ListTaskIDsWithUnmetCompletionCriteria(ctx, query)
		if err != nil {
			return err
		}
		for _, taskID := range ids {
			visit(taskID)
		}
		if len(ids) < query.Limit {
			return nil
		}
		query.AfterTaskID = ids[len(ids)-1]
	}
	return ctx.Err()
}

func (v *CIVerifier) evaluateLogged(ctx context.Context, taskID string) {
	if err := v.Evaluate(ctx, taskID); err != nil && ctx.Err() == nil {
		v.logger.Warn("completion gate CI evaluation failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

// Evaluate reads the task's linked PRs and records one CI result.
func (v *CIVerifier) Evaluate(ctx context.Context, taskID string) error {
	snapshot, err := v.gates.GetTaskCompletionGate(ctx, taskID)
	if err != nil {
		return err
	}
	criterion, ok := findCriterionOfKind(snapshot, models.TaskCompletionCriterionCIAtHead)
	if !ok || criterion.HeldReason == HoldAttemptCapReached || criterion.HeldReason == HoldReworkCapReached {
		return nil
	}
	links, err := v.prs.LinkedPRs(ctx, taskID)
	if err != nil || len(links) == 0 {
		return err
	}
	if criterion.ResultReason == models.TaskCompletionReasonSentBack && criterion.ResultSubjectRevision == HeadsRevision(links) {
		return nil
	}
	input, current, err := v.observe(ctx, taskID, criterion, links)
	if err != nil || !current {
		return err
	}
	return v.record(ctx, taskID, criterion, input, aggregateCI(input))
}

// observe reads checks (and files when paths are protected) for every link.
// current is false when a head moved during the reads.
func (v *CIVerifier) observe(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion, links []PRLink) (ciInput, bool, error) {
	input := ciInput{criterion: criterion, revision: HeadsRevision(links), now: v.now()}
	if criterion.ResultSubjectRevision == input.revision {
		input.firstSeenAt = criterion.ResultFirstSeenAt
	}
	for _, link := range links {
		checks, err := v.prs.ObserveChecks(ctx, taskID, link)
		if err != nil {
			return input, false, err
		}
		if checks.HeadSHA != link.HeadSHA {
			return input, false, nil
		}
		input.checks = append(input.checks, checks)
		if len(criterion.Config.ProtectedPaths) == 0 {
			continue
		}
		files, err := v.prs.ChangedFiles(ctx, taskID, link)
		if err != nil {
			// An unreadable file list cannot clear protected paths at this head.
			v.logger.Warn("completion gate changed-file read failed", zap.String("task_id", taskID), zap.Error(err))
			input.files = append(input.files, FilesObservation{HeadSHA: link.HeadSHA})
			continue
		}
		if files.HeadSHA != link.HeadSHA {
			return input, false, nil
		}
		input.files = append(input.files, files)
	}
	return input, true, nil
}

func (v *CIVerifier) record(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion, input ciInput, outcome ciOutcome) error {
	change := models.TaskCompletionEvaluationChange{
		TaskID: taskID, CriterionID: criterion.ID, CriterionRevision: criterion.CriterionRevision,
		SubjectRevision: input.revision, Result: outcome.result, Reason: outcome.reason, Detail: outcome.detail,
		VerifierKind: VerifierKindCI, VerifierID: VerifierKindCI, Hold: outcome.hold,
	}
	if outcome.hold == "" {
		change.ReleaseHold = HoldProtectedPathChanged
	}
	if outcome.result == models.TaskCompletionResultPass {
		change.EvidenceSummary = fmt.Sprintf("%d pull request(s) passed CI", len(input.checks))
	}
	policy := v.currentPolicy()
	if policy != nil {
		policy.Prepare(ctx, taskID, criterion, &change)
	}
	result, err := v.gates.RecordTaskCompletionEvaluation(ctx, change)
	if errors.Is(err, repoerrors.ErrTaskCompletionEvidenceChanged) {
		return nil
	}
	if err != nil {
		return err
	}
	if !result.Changed {
		return nil
	}
	recordEvaluationMetric(models.TaskCompletionCriterionCIAtHead, outcome.result)
	if change.Hold != "" && result.Criterion.HeldReason == change.Hold {
		recordHoldMetric(change.Hold)
	}
	v.logger.Info("completion gate CI result recorded", zap.String("task_id", taskID),
		zap.String("result", outcome.result), zap.String("reason", outcome.reason))
	if policy != nil {
		policy.AfterRecord(ctx, taskID, result)
	}
	return nil
}
