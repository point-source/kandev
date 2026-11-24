package orchestrator

import (
	"context"

	"go.uber.org/zap"
)

// SetAssigneeStartHold installs the check that withholds a workflow step's
// auto-start from an agent whose task completion gate holds it.
func (s *Service) SetAssigneeStartHold(held func(ctx context.Context, taskID, agentProfileID string) bool) {
	s.assigneeStartHeld = held
}

func (s *Service) autoStartHeld(ctx context.Context, taskID, sessionID string) bool {
	if s.assigneeStartHeld == nil {
		return false
	}
	session, err := s.repo.GetTaskSession(ctx, sessionID)
	if err != nil || session == nil || !s.assigneeStartHeld(ctx, taskID, session.AgentProfileID) {
		return false
	}
	s.logger.Info("workflow auto-start withheld by a completion gate hold",
		zap.String("task_id", taskID), zap.String("session_id", sessionID))
	return true
}
