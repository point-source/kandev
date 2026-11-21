package backendapp

import (
	"context"
	"fmt"
	"sort"

	"github.com/kandev/kandev/internal/common/logger"
	"github.com/kandev/kandev/internal/events/bus"
	githubpkg "github.com/kandev/kandev/internal/github"
	"github.com/kandev/kandev/internal/office/completiongates"
	officemodels "github.com/kandev/kandev/internal/office/models"
	officeservice "github.com/kandev/kandev/internal/office/service"
	taskservice "github.com/kandev/kandev/internal/task/service"
	workflowmodels "github.com/kandev/kandev/internal/workflow/models"
	v1 "github.com/kandev/kandev/pkg/api/v1"
)

// completionGatePRSource adapts the GitHub service to the Office completion
// gate verifiers without exposing GitHub types to Office.
type completionGatePRSource struct {
	github *githubpkg.Service
}

func (a completionGatePRSource) linkedRows(ctx context.Context, taskID string) ([]*githubpkg.TaskPR, error) {
	byTask, err := a.github.ListTaskPRsByTaskIDs(ctx, []string{taskID})
	if err != nil {
		return nil, err
	}
	rows := make([]*githubpkg.TaskPR, 0, len(byTask[taskID]))
	for _, row := range byTask[taskID] {
		if row != nil && row.DetachedAt == nil {
			rows = append(rows, row)
		}
	}
	return rows, nil
}

func (a completionGatePRSource) LinkedPRs(ctx context.Context, taskID string) ([]completiongates.PRLink, error) {
	rows, err := a.linkedRows(ctx, taskID)
	if err != nil {
		return nil, err
	}
	links := make([]completiongates.PRLink, len(rows))
	for index, row := range rows {
		links[index] = completiongates.PRLink{
			ID: row.ID, Owner: row.Owner, Repo: row.Repo, Number: row.PRNumber, HeadSHA: row.HeadSHA, State: row.State,
		}
	}
	return links, nil
}

func (a completionGatePRSource) row(ctx context.Context, taskID string, link completiongates.PRLink) (*githubpkg.TaskPR, error) {
	rows, err := a.linkedRows(ctx, taskID)
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if row.ID == link.ID {
			return row, nil
		}
	}
	return nil, fmt.Errorf("linked pull request %s is no longer attached", link.ID)
}

func (a completionGatePRSource) ObserveChecks(ctx context.Context, taskID string, link completiongates.PRLink) (completiongates.CheckObservation, error) {
	row, err := a.row(ctx, taskID, link)
	if err != nil {
		return completiongates.CheckObservation{}, err
	}
	observed, err := a.github.ObserveChecksAtHead(ctx, row)
	if err != nil {
		return completiongates.CheckObservation{}, err
	}
	return completiongates.CheckObservation{
		HeadSHA: observed.HeadSHA, State: observed.State, Observed: observed.Observed, Skipped: observed.Skipped,
		FailingNames: observed.FailingNames, ActiveWorkflowRuns: observed.ActiveWorkflowRuns,
	}, nil
}

func (a completionGatePRSource) ChangedFiles(ctx context.Context, taskID string, link completiongates.PRLink) (completiongates.FilesObservation, error) {
	row, err := a.row(ctx, taskID, link)
	if err != nil {
		return completiongates.FilesObservation{}, err
	}
	observed, err := a.github.ChangedFilesAtHead(ctx, row)
	if err != nil {
		return completiongates.FilesObservation{}, err
	}
	return completiongates.FilesObservation{HeadSHA: observed.HeadSHA, Paths: observed.Paths, Complete: observed.Complete}, nil
}

// buildCompletionGatesService returns nil when GitHub is unavailable; gated
// tasks then keep a visible blocker and no verifier runs.
func buildCompletionGatesService(
	taskSvc *taskservice.Service, githubSvc *githubpkg.Service, officeSvc *officeservice.Service,
	repos *Repositories, eventBus bus.EventBus, log *logger.Logger,
) *completiongates.Service {
	if taskSvc == nil || githubSvc == nil || officeSvc == nil {
		return nil
	}
	return completiongates.NewService(completiongates.Dependencies{
		Gates: taskSvc, PRs: completionGatePRSource{github: githubSvc}, Tasks: taskSvc,
		Reviews: repos.Workflow, Settings: repos.Office, Steps: completionGateStepCaps{steps: repos.Workflow},
		Agents: completionGateAgents{office: officeSvc}, Runs: completionGateRuns{office: officeSvc},
		History: taskSvc, Held: taskSvc, Stages: completionGateStepCaps{steps: repos.Workflow}, Ledger: taskSvc,
		Bus: eventBus, Logger: log,
	})
}

// completionGateNavigator reads workflow placement and moves tasks through
// the task service for the completion-gate router.
type completionGateNavigator struct {
	tasks      *taskservice.Service
	steps      workflowStepLister
	dispatcher guardedTransitionRetrier
}

type workflowStepLister interface {
	ListStepsByWorkflow(ctx context.Context, workflowID string) ([]*workflowmodels.WorkflowStep, error)
}

type guardedTransitionRetrier interface {
	RetryGuardedTransitions(ctx context.Context, taskID, stepID, operationID string, accept func(string) bool) (bool, error)
}

func (n completionGateNavigator) TaskPlacement(ctx context.Context, taskID string) (completiongates.Placement, error) {
	task, err := n.tasks.GetTask(ctx, taskID)
	if err != nil {
		return completiongates.Placement{}, err
	}
	placement := completiongates.Placement{
		WorkflowID: task.WorkflowID, CurrentStepID: task.WorkflowStepID,
		Terminal: placementTerminal(task.State),
	}
	steps, err := n.steps.ListStepsByWorkflow(ctx, task.WorkflowID)
	if err != nil {
		return placement, err
	}
	applyStepPlacement(&placement, steps)
	return placement, nil
}

// placementTerminal reports a finished task. A failed run leaves an Office
// task recoverable, so a failed task is still routed.
func placementTerminal(state v1.TaskState) bool {
	return state == v1.TaskStateCompleted || state == v1.TaskStateCancelled
}

// applyStepPlacement records the current position and the first work and
// review stages by position.
func applyStepPlacement(placement *completiongates.Placement, steps []*workflowmodels.WorkflowStep) {
	sort.SliceStable(steps, func(i, j int) bool { return steps[i].Position < steps[j].Position })
	for _, step := range steps {
		if step.ID == placement.CurrentStepID {
			placement.CurrentPosition = step.Position
		}
		switch {
		case step.StageType == workflowmodels.StageTypeWork && placement.WorkStepID == "":
			placement.WorkStepID, placement.WorkPosition = step.ID, step.Position
		case step.StageType == workflowmodels.StageTypeReview && placement.ReviewStepID == "":
			placement.ReviewStepID, placement.ReviewPosition = step.ID, step.Position
		}
	}
}

func (n completionGateNavigator) MoveTask(ctx context.Context, taskID, workflowID, stepID string) error {
	_, err := n.tasks.MoveTask(ctx, taskID, workflowID, stepID, 0)
	return err
}

// RetryCompletingTransition re-applies only a satisfied guarded transition
// into the workflow's completing step; other guarded moves stay refused.
func (n completionGateNavigator) RetryCompletingTransition(ctx context.Context, taskID, workflowID, stepID, operationID string) error {
	if n.dispatcher == nil {
		return nil
	}
	steps, err := n.steps.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return err
	}
	completing := completingStepIDs(steps)
	if len(completing) == 0 {
		return nil
	}
	_, err = n.dispatcher.RetryGuardedTransitions(ctx, taskID, stepID, operationID, func(target string) bool {
		return completing[target]
	})
	return err
}

// completingStepIDs returns the steps that complete their task on entry: those
// configured to, with no step after them by position.
func completingStepIDs(steps []*workflowmodels.WorkflowStep) map[string]bool {
	last := 0
	for index, step := range steps {
		if index == 0 || step.Position > last {
			last = step.Position
		}
	}
	completing := map[string]bool{}
	for _, step := range steps {
		if step.Position == last && step.CompleteTaskOnEnter {
			completing[step.ID] = true
		}
	}
	return completing
}

// completionGateAgents resolves escalation recipients from Office agents.
type completionGateAgents struct {
	office *officeservice.Service
}

func (a completionGateAgents) ActiveAgent(ctx context.Context, workspaceID, agentID string) bool {
	agent, err := a.office.GetAgentFromConfig(ctx, agentID)
	if err != nil || agent == nil || agent.WorkspaceID != workspaceID {
		return false
	}
	return agent.Status != officemodels.AgentStatusPaused && agent.Status != officemodels.AgentStatusStopped &&
		agent.Status != officemodels.AgentStatusPendingApproval
}

func (a completionGateAgents) WorkspaceCEO(ctx context.Context, workspaceID string) string {
	ceos, err := a.office.ListAgentInstancesFiltered(ctx, workspaceID, officeservice.AgentListFilter{Role: string(officemodels.AgentRoleCEO)})
	if err != nil || len(ceos) == 0 {
		return ""
	}
	return ceos[0].ID
}

// completionGateRuns queues escalation runs through the Office run queue.
type completionGateRuns struct {
	office *officeservice.Service
}

func (a completionGateRuns) QueueRun(ctx context.Context, agentID, reason, payload, idempotencyKey string) error {
	_, err := a.office.QueueRun(ctx, agentID, reason, payload, idempotencyKey)
	return err
}

// completionGateStepCaps reads the cap defaults of a workflow's completing step.
type completionGateStepCaps struct {
	steps workflowStepLister
}

func (a completionGateStepCaps) CompletingStepCaps(ctx context.Context, workflowID string) (int, int, error) {
	if workflowID == "" {
		return 0, 0, nil
	}
	steps, err := a.steps.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return 0, 0, err
	}
	var final *workflowmodels.WorkflowStep
	for _, step := range steps {
		if final == nil || step.Position > final.Position {
			final = step
		}
	}
	if final == nil || !final.CompleteTaskOnEnter || final.CompletionGateCaps == nil {
		return 0, 0, nil
	}
	return final.CompletionGateCaps.MaxVerificationAttempts, final.CompletionGateCaps.MaxReworkRounds, nil
}

// HasReviewStage reports whether a workflow has a review-stage step.
func (a completionGateStepCaps) HasReviewStage(ctx context.Context, workflowID string) (bool, error) {
	if workflowID == "" {
		return false, nil
	}
	steps, err := a.steps.ListStepsByWorkflow(ctx, workflowID)
	if err != nil {
		return false, err
	}
	for _, step := range steps {
		if step.StageType == workflowmodels.StageTypeReview {
			return true, nil
		}
	}
	return false, nil
}
