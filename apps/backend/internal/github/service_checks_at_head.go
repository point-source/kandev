package github

import (
	"context"
	"fmt"
	"strings"
)

// githubPRFilesLimit is the most files GitHub lists for one pull request.
const githubPRFilesLimit = 3000

// CheckObservation is the selected-check state of one PR at its current head,
// using the same selection as PR status.
type CheckObservation struct {
	HeadSHA            string
	State              string
	Observed           int
	Skipped            int
	FailingNames       []string
	ActiveWorkflowRuns bool
}

// ChangedFilesObservation lists every path a PR touches at one head. Paths
// include both sides of a rename. Complete is false when the list may be
// truncated or the head moved while it was read.
type ChangedFilesObservation struct {
	HeadSHA  string
	Paths    []string
	Complete bool
}

// ObserveChecksAtHead reads the selected checks of a linked PR at its live
// head. It bypasses the feedback cache because completion gates need a
// current observation.
func (s *Service) ObserveChecksAtHead(ctx context.Context, tp *TaskPR) (*CheckObservation, error) {
	client, ctx, err := s.taskPRClient(ctx, tp)
	if err != nil {
		return nil, err
	}
	pr, err := client.GetPR(ctx, tp.Owner, tp.Repo, tp.PRNumber)
	if err != nil {
		return nil, err
	}
	checks, err := client.ListCheckRuns(ctx, tp.Owner, tp.Repo, pr.HeadSHA)
	if err != nil {
		return nil, err
	}
	workflowObservation, workflowErr := collectWorkflowObservation(ctx, client, tp.Owner, tp.Repo, pr)
	observation := observeSelectedChecks(pr, checks, workflowObservation.Runs)
	if workflowErr != nil {
		markWorkflowsUnknown(observation)
	}
	return observation, nil
}

// markWorkflowsUnknown treats unreadable workflow runs as possibly active, so
// an unfinished workflow is never mistaken for missing or passing checks.
func markWorkflowsUnknown(observation *CheckObservation) {
	observation.ActiveWorkflowRuns = true
	if observation.State != checkConclusionFail {
		observation.State = checkStatusPending
	}
}

func observeSelectedChecks(pr *PR, checks []CheckRun, runs []WorkflowRun) *CheckObservation {
	selection := selectCurrentPRChecks(pr, checks, runs)
	summary := summarizeSelectedChecks(pr.HeadSHA, selection.Checks)
	observation := &CheckObservation{
		HeadSHA:            pr.HeadSHA,
		State:              selectedPRChecksState(selection.Checks, selection.ActiveWorkflowRuns),
		Observed:           summary.Observed,
		Skipped:            summary.Skipped,
		ActiveWorkflowRuns: selection.ActiveWorkflowRuns,
	}
	for _, check := range selection.Checks {
		if hasFailingChecks([]CheckRun{check}) {
			observation.FailingNames = append(observation.FailingNames, check.Name)
		}
	}
	return observation
}

// ChangedFilesAtHead lists the files a linked PR touches and reports whether
// the list is complete for one unchanged head.
func (s *Service) ChangedFilesAtHead(ctx context.Context, tp *TaskPR) (*ChangedFilesObservation, error) {
	client, ctx, err := s.taskPRClient(ctx, tp)
	if err != nil {
		return nil, err
	}
	before, err := client.GetPR(ctx, tp.Owner, tp.Repo, tp.PRNumber)
	if err != nil {
		return nil, err
	}
	files, err := client.ListPRFiles(ctx, tp.Owner, tp.Repo, tp.PRNumber)
	if err != nil {
		return nil, err
	}
	after, err := client.GetPR(ctx, tp.Owner, tp.Repo, tp.PRNumber)
	if err != nil {
		return nil, err
	}
	return changedFilesObservation(before, after, files), nil
}

func changedFilesObservation(before, after *PR, files []PRFile) *ChangedFilesObservation {
	observation := &ChangedFilesObservation{HeadSHA: after.HeadSHA}
	for _, file := range files {
		observation.Paths = append(observation.Paths, file.Filename)
		if file.OldPath != "" && file.OldPath != file.Filename {
			observation.Paths = append(observation.Paths, file.OldPath)
		}
	}
	observation.Complete = before.HeadSHA == after.HeadSHA && after.ChangedFilesObserved &&
		len(files) == after.ChangedFiles && len(files) < githubPRFilesLimit
	return observation
}

// taskPRClient resolves the workspace automation client for a linked PR.
func (s *Service) taskPRClient(ctx context.Context, tp *TaskPR) (Client, context.Context, error) {
	if tp == nil {
		return nil, ctx, fmt.Errorf("task PR is required")
	}
	workspaceID := strings.TrimSpace(tp.WorkspaceID)
	if workspaceID == "" {
		resolvedWorkspace, err := s.resolveTaskWatchWorkspace(ctx, tp.TaskID)
		if err != nil {
			return nil, ctx, err
		}
		workspaceID = resolvedWorkspace
	}
	resolved, err := s.resolveAutomationClient(ctx, workspaceID, tp.Owner, tp.Repo)
	if err != nil {
		return nil, ctx, err
	}
	return resolved.Client, withGitHubOperationFence(ctx, s, workspaceID, resolved), nil
}
