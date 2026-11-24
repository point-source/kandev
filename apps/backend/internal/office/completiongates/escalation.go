package completiongates

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"go.uber.org/zap"

	"github.com/kandev/kandev/internal/common/logger"
	officemodels "github.com/kandev/kandev/internal/office/models"
	"github.com/kandev/kandev/internal/office/shared"
	"github.com/kandev/kandev/internal/task/models"
)

// RunReasonCompletionGateHeld wakes a criterion owner about a hold.
const RunReasonCompletionGateHeld = shared.RunReasonTaskCompletionGateHeld

// RunReasonChangesRequested wakes the assignee with a person's send-back reason.
const RunReasonChangesRequested = shared.RunReasonTaskChangesRequested

// Owner kinds recorded on criteria.
const (
	ownerKindAgent = "agent"
	ownerKindHuman = "human"
)

// AgentDirectory resolves escalation recipients.
type AgentDirectory interface {
	// ActiveAgent reports whether agentID is an active agent of the workspace.
	ActiveAgent(ctx context.Context, workspaceID, agentID string) bool
	// WorkspaceCEO returns the first active CEO agent, or "".
	WorkspaceCEO(ctx context.Context, workspaceID string) string
}

// RunQueue queues an idempotent agent run.
type RunQueue interface {
	QueueRun(ctx context.Context, agentID, reason, payload, idempotencyKey string) error
}

// HistoryReader reads a task's gate history for the escalation payload.
type HistoryReader interface {
	ListTaskCompletionGateHistory(ctx context.Context, taskID string) ([]*models.TaskCompletionGateHistory, error)
}

// Escalator wakes a criterion's owner agent once per hold and records the
// escalation. Holds owned by a person, or that could not be escalated, reach
// people through the Inbox.
type Escalator struct {
	gates   GateService
	tasks   TaskReader
	agents  AgentDirectory
	runs    RunQueue
	history HistoryReader
	logger  *logger.Logger
}

// NewEscalator creates the hold escalator.
func NewEscalator(gates GateService, tasks TaskReader, agents AgentDirectory, runs RunQueue, history HistoryReader, log *logger.Logger) *Escalator {
	return &Escalator{gates: gates, tasks: tasks, agents: agents, runs: runs, history: history, logger: log}
}

// EscalateHold queues one wake for the hold's recipient agent, then marks the
// hold escalated. A failed wake stays unmarked so the sweep retries it.
func (e *Escalator) EscalateHold(ctx context.Context, taskID string, criterion models.TaskCompletionCriterion) {
	if criterion.HoldKind == "" || criterion.HoldKind == HoldProtectedFilesChanged || criterion.EscalatedAt != nil {
		return
	}
	task, err := e.tasks.GetTask(ctx, taskID)
	if err != nil {
		return
	}
	recipient := e.recipient(ctx, task.WorkspaceID, criterion)
	if recipient == "" {
		return
	}
	heldAt := ""
	if criterion.HeldAt != nil {
		heldAt = criterion.HeldAt.UTC().Format(time.RFC3339Nano)
	}
	payload, _ := json.Marshal(map[string]string{
		"task_id": taskID, "criterion_id": criterion.ID, "criterion_kind": criterion.Kind,
		"hold_kind": criterion.HoldKind, "history": strings.Join(e.historyLines(ctx, taskID), "\n"),
	})
	key := "completion_gate_held:" + taskID + ":" + criterion.ID + ":" + heldAt
	if err := e.runs.QueueRun(ctx, recipient, RunReasonCompletionGateHeld, string(payload), key); err != nil {
		e.logger.Warn("completion gate hold escalation failed", zap.String("task_id", taskID), zap.Error(err))
		return
	}
	if _, err := e.gates.ApplyTaskCompletionControl(ctx, models.TaskCompletionControlChange{
		TaskID: taskID, CriterionID: criterion.ID, CriterionRevision: criterion.CriterionRevision,
		Action: models.TaskCompletionControlMarkEscalated, ActorKind: "system", ActorID: "completion_gate_escalator",
	}); err != nil {
		e.logger.Warn("completion gate escalation mark failed", zap.String("task_id", taskID), zap.Error(err))
	}
}

// sweep retries escalation of every held criterion not yet escalated.
func (e *Escalator) sweep(ctx context.Context) error {
	query := models.TaskCompletionSweepQuery{HeldUnescalated: true, Limit: sweepBatchSize}
	return forEachSweepPage(ctx, e.gates, query, func(taskID string) {
		snapshot, err := e.gates.GetTaskCompletionGate(ctx, taskID)
		if err != nil || snapshot == nil {
			return
		}
		for _, criterion := range snapshot.Criteria {
			if models.IsHostVerifiedCriterionKind(criterion.Kind) {
				e.EscalateHold(ctx, taskID, criterion)
			}
		}
	})
}

// recipient is the owner agent when active, else the workspace CEO. A
// person-owned criterion has no agent recipient.
func (e *Escalator) recipient(ctx context.Context, workspaceID string, criterion models.TaskCompletionCriterion) string {
	if !ownedByAgentOrRoutine(criterion) {
		return ""
	}
	if criterion.OwnerKind == ownerKindAgent && e.agents.ActiveAgent(ctx, workspaceID, criterion.OwnerID) {
		return criterion.OwnerID
	}
	return e.agents.WorkspaceCEO(ctx, workspaceID)
}

func ownedByAgentOrRoutine(criterion models.TaskCompletionCriterion) bool {
	return criterion.OwnerKind != ownerKindHuman && criterion.OwnerKind != ""
}

const maxHistoryLines = 20

func (e *Escalator) historyLines(ctx context.Context, taskID string) []string {
	if e.history == nil {
		return nil
	}
	entries, err := e.history.ListTaskCompletionGateHistory(ctx, taskID)
	if err != nil {
		return nil
	}
	if len(entries) > maxHistoryLines {
		entries = entries[len(entries)-maxHistoryLines:]
	}
	lines := make([]string, 0, len(entries))
	for _, entry := range entries {
		lines = append(lines, entry.CreatedAt.UTC().Format(time.RFC3339)+" "+entry.Action+" "+entry.Reason)
	}
	return lines
}

// InboxTypeHold is the Inbox item type of a held completion criterion. Its
// payload names the hold kind, which decides the item's title and actions.
const InboxTypeHold = "completion_gate_hold"

// HeldLister lists held criteria of a workspace.
type HeldLister interface {
	ListHeldCompletionCriteria(ctx context.Context, workspaceID string) ([]models.HeldTaskCompletionCriterion, error)
}

// InboxTypeApproval is the Inbox item type of an approval that asks a person.
const InboxTypeApproval = "completion_gate_approval"

func inboxItems(held []models.HeldTaskCompletionCriterion) []*officemodels.InboxItem {
	items := make([]*officemodels.InboxItem, 0, len(held))
	for _, entry := range held {
		criterion := entry.Criterion
		if criterion.HoldKind == "" {
			if criterion.ResultReason == models.TaskCompletionReasonAwaitingPerson {
				items = append(items, approvalInboxItem(entry))
			}
			continue
		}
		if criterion.HoldKind != HoldProtectedFilesChanged && ownedByAgentOrRoutine(criterion) && criterion.EscalatedAt != nil {
			continue
		}
		created := time.Now().UTC()
		if criterion.HeldAt != nil {
			created = *criterion.HeldAt
		}
		items = append(items, &officemodels.InboxItem{
			ID: "completion-gate:" + entry.TaskID + ":" + criterion.ID, Type: InboxTypeHold,
			Title: entry.TaskTitle, Description: strings.Join(criterion.ResultDetail, ", "),
			Status: "pending", EntityID: entry.TaskID, EntityType: "task",
			Payload: map[string]any{
				"task_id": entry.TaskID, "criterion_id": criterion.ID, "criterion_kind": criterion.Kind,
				"criterion_revision": criterion.CriterionRevision, "hold_kind": criterion.HoldKind,
				"detail": criterion.ResultDetail, "subject_revision": criterion.ResultSubjectRevision,
				"rework_count": criterion.ReworkCount, "attempt_count": criterion.AttemptCount,
				"held_at": created.UTC().Format(time.RFC3339Nano),
			},
			CreatedAt: created,
		})
	}
	return items
}

// approvalInboxItem asks a person to approve the task's live heads.
func approvalInboxItem(entry models.HeldTaskCompletionCriterion) *officemodels.InboxItem {
	criterion := entry.Criterion
	asked := time.Now().UTC()
	if criterion.ResultAt != nil {
		asked = *criterion.ResultAt
	}
	return &officemodels.InboxItem{
		ID: "completion-gate:" + entry.TaskID + ":" + criterion.ID, Type: InboxTypeApproval,
		Title: entry.TaskTitle, Status: "pending", EntityID: entry.TaskID, EntityType: "task",
		Payload: map[string]any{
			"task_id": entry.TaskID, "criterion_id": criterion.ID, "criterion_kind": criterion.Kind,
			"criterion_revision": criterion.CriterionRevision, "subject_revision": criterion.ResultSubjectRevision,
			"rework_count": criterion.ReworkCount, "asked_at": asked.UTC().Format(time.RFC3339Nano),
		},
		CreatedAt: asked,
	}
}
