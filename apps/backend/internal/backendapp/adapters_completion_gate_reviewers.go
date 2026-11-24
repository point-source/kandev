package backendapp

import (
	"context"
	"encoding/json"

	"github.com/kandev/kandev/internal/office/completiongates"
	"github.com/kandev/kandev/internal/task/models"
	"github.com/kandev/kandev/internal/workflow/engine"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
)

// reseatRunReason is the wake reason of a reviewer seated after the review
// stage was entered; it matches the review step's own seat fan-out.
const reseatRunReason = "task_assigned"

type reviewerCaster interface {
	CastParticipantSeat(ctx context.Context, workflowID, taskID, stepID, role string) (engine.ParticipantSeatCastResult, error)
}

type taskReviewerAdder interface {
	AddTaskReviewer(ctx context.Context, callerAgentID, taskID, agentID string) error
}

type completionGateTaskReader interface {
	GetTask(ctx context.Context, taskID string) (*models.Task, error)
}

// completionGateReviewerSeater seats a reviewer that can approve a held
// review: it casts the reviewer role again, and when the cast picks an agent
// other than the assignee it registers that agent as the task's reviewer
// (claiming the automatic fallback seat when it can) and queues its review
// run at the review step.
type completionGateReviewerSeater struct {
	tasks     completionGateTaskReader
	steps     workflowStepLister
	caster    reviewerCaster
	reviewers taskReviewerAdder
	runs      completiongates.RunQueue
}

func (a completionGateReviewerSeater) SeatReviewer(ctx context.Context, taskID string) (bool, error) {
	task, err := a.tasks.GetTask(ctx, taskID)
	if err != nil {
		return false, err
	}
	step, err := a.reviewStep(ctx, task.WorkflowID)
	if err != nil || step == nil || task.WorkflowStepID != step.ID {
		return false, err
	}
	cast, err := a.caster.CastParticipantSeat(ctx, task.WorkflowID, taskID, step.ID, string(workflowmodels.ParticipantRoleReviewer))
	if err != nil || cast.Unfillable || cast.AgentProfileID == "" || cast.AgentProfileID == task.AssigneeAgentProfileID {
		return false, err
	}
	if err := a.reviewers.AddTaskReviewer(ctx, "", taskID, cast.AgentProfileID); err != nil {
		return false, err
	}
	// The seat lands at the task's current step; a task that left Review in
	// the meantime keeps its hold and gets a new cast when it returns.
	if after, err := a.tasks.GetTask(ctx, taskID); err != nil || after.WorkflowStepID != step.ID {
		return false, err
	}
	payload, _ := json.Marshal(map[string]string{"task_id": taskID, "workflow_step_id": step.ID})
	key := "completion_gate_reseat:" + taskID + ":" + cast.AgentProfileID
	if err := a.runs.QueueRun(ctx, cast.AgentProfileID, reseatRunReason, string(payload), key); err != nil {
		return false, err
	}
	return true, nil
}

// reviewStep returns the workflow's first review-stage step.
func (a completionGateReviewerSeater) reviewStep(ctx context.Context, workflowID string) (*workflowmodels.WorkflowStep, error) {
	steps, err := a.steps.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return nil, err
	}
	var review *workflowmodels.WorkflowStep
	for _, step := range steps {
		if step.StageType == workflowmodels.StageTypeReview && (review == nil || step.Position < review.Position) {
			review = step
		}
	}
	return review, nil
}
