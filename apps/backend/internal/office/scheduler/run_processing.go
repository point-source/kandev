package scheduler

import (
	"context"
	"database/sql"
	"errors"

	runsmodels "github.com/kandev/kandev/internal/runs/models"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/service"
)

// ClaimNextRun atomically claims the next eligible run from the queue.
// Returns nil, nil if no run is available.
func (ss *SchedulerService) ClaimNextRun(ctx context.Context) (*runsmodels.Run, error) {
	req, err := ss.repo.ClaimNextEligibleRun(ctx)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	ss.logger.Info("run claimed",
		zap.String("id", req.ID),
		zap.String("agent", req.AgentProfileID),
		zap.String("reason", req.Reason))
	return req, nil
}

// FinishRun marks a claimed run as finished. outcome is written as NULL:
// this dormant parallel copy of the service-layer FinishRun has no
// production caller and therefore no established semantic label for what
// happened, so it is not bucketed as processed (docs/specs/
// task-delivery-ledger/spec.md, "Office run outcome").
func (ss *SchedulerService) FinishRun(ctx context.Context, id string) error {
	_, err := ss.repo.FinishRun(ctx, id, RunStatusFinished, nil)
	return err
}

// FailRun marks a claimed run as failed. outcome is written as NULL, same
// as the service-layer FailRun: a failed run is bucketed on status alone.
func (ss *SchedulerService) FailRun(ctx context.Context, id string) error {
	_, err := ss.repo.FinishRun(ctx, id, RunStatusFailed, nil)
	return err
}

// ProcessRunGuard checks if the agent is still eligible to be woken.
// Returns true if the run should proceed, false if it should be skipped.
func (ss *SchedulerService) ProcessRunGuard(ctx context.Context, run *runsmodels.Run) (bool, error) {
	agent, err := ss.svc.GetAgentFromConfig(ctx, run.AgentProfileID)
	if err != nil {
		return false, err
	}
	switch agent.Status {
	case models.AgentStatusPaused, models.AgentStatusStopped, models.AgentStatusPendingApproval:
		ss.logger.Info("run skipped (agent not active)",
			zap.String("run_id", run.ID),
			zap.String("agent_status", string(agent.Status)))
		return false, nil
	}
	if ss.completionHolds != nil {
		if taskID := service.ParseRunPayload(run.Payload)["task_id"]; ss.completionHolds.AssigneeRunHeld(ctx, taskID, run.AgentProfileID) {
			ss.logger.Info("run skipped (completion_gate_held)",
				zap.String("run_id", run.ID), zap.String("task_id", taskID))
			return false, nil
		}
	}
	return true, nil
}

// CompletionHoldChecker reports whether a run's agent is the assignee of a
// task with a held completion criterion (REQ-OFFICE-COMPLETION-GATES-007).
type CompletionHoldChecker interface {
	AssigneeRunHeld(ctx context.Context, taskID, agentID string) bool
}

// SetCompletionHoldChecker installs the completion-gate run guard.
func (ss *SchedulerService) SetCompletionHoldChecker(checker CompletionHoldChecker) {
	ss.completionHolds = checker
}
