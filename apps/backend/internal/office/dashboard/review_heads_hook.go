package dashboard

import (
	"context"

	"github.com/kandev/kandev/internal/office/models"
)

// ReviewHeadsHook pins review decisions to the task's PR heads for
// completion criteria (REQ-OFFICE-COMPLETION-GATES-004).
type ReviewHeadsHook interface {
	// DecisionHeads returns the heads revision to store with a decision, and
	// a notice when the decision cannot count toward a criterion.
	DecisionHeads(ctx context.Context, taskID, deciderType, deciderID string) (heads, notice string)
	// DecisionRecorded runs after a decision is committed.
	DecisionRecorded(ctx context.Context, taskID, stepID, decision string)
}

// SetReviewHeadsHook installs the completion-gate review hook.
func (s *DashboardService) SetReviewHeadsHook(hook ReviewHeadsHook) {
	s.reviewHeads = hook
}

func (s *DashboardService) reviewHeadsForDecision(ctx context.Context, taskID, deciderType, deciderID string) (string, string) {
	if s.reviewHeads == nil {
		return "", ""
	}
	return s.reviewHeads.DecisionHeads(ctx, taskID, deciderType, deciderID)
}

func (s *DashboardService) notifyReviewDecisionRecorded(ctx context.Context, taskID, stepID, decision string) {
	if s.reviewHeads != nil {
		s.reviewHeads.DecisionRecorded(ctx, taskID, stepID, decision)
	}
}

// CompletionGateInbox supplies completion-gate Inbox items.
type CompletionGateInbox interface {
	InboxItems(ctx context.Context, workspaceID string) ([]*models.InboxItem, error)
}

// SetCompletionGateInbox installs the completion-gate Inbox source.
func (s *DashboardService) SetCompletionGateInbox(source CompletionGateInbox) {
	s.completionGateInbox = source
}

func (s *DashboardService) inboxCompletionGateItems(ctx context.Context, workspaceID string) ([]*models.InboxItem, error) {
	if s.completionGateInbox == nil {
		return nil, nil
	}
	return s.completionGateInbox.InboxItems(ctx, workspaceID)
}
