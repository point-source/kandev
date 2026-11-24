import type { ApiClient, MockCheckRun } from "./api-client";

export const GATE_OWNER = "acme";
export const GATE_REPO = "gated-app";
export const GATE_PR = 4242;

type Seed = { workspaceId: string; workflowId: string };

type GateKind = "check" | "review" | "approval";

/**
 * Creates an Office task in the work step carrying the given criteria: a
 * GitHub check (with optional protected paths), a review and/or a human
 * approval.
 */
export async function seedGatedTask(
  apiClient: ApiClient,
  seed: Seed,
  title: string,
  protectedPaths: string[] = [],
  kinds: GateKind[] = ["check"],
): Promise<string> {
  const { steps } = await apiClient.listWorkflowSteps(seed.workflowId);
  const work = steps.find((step) => step.stage_type === "work") ?? steps[0];
  const task = await apiClient.createTask(seed.workspaceId, title, {
    workflow_id: seed.workflowId,
    workflow_step_id: work.id,
  });
  const criteria = kinds.map((kind) => ({
    id: kind === "check" ? "github_check" : kind,
    description: "",
    kind,
    config:
      kind === "check"
        ? {
            runner: "github",
            ...(protectedPaths.length ? { protected_paths: protectedPaths } : {}),
          }
        : {},
    evidence_subject: { kind: "github_pr_heads", id: task.id },
    criterion_revision: 0,
  }));
  const response = await apiClient.rawRequest(
    "PUT",
    `/api/v1/tasks/${task.id}/completion-gate/criteria`,
    { expected_revision: 0, criteria },
  );
  if (!response.ok)
    throw new Error(`set criteria failed: ${response.status} ${await response.text()}`);
  return task.id;
}

/** Moves a task to the workflow's review step, which seats its reviewers. */
export async function moveToReviewStep(apiClient: ApiClient, seed: Seed, taskId: string) {
  const { steps } = await apiClient.listWorkflowSteps(seed.workflowId);
  const review = steps.find((step) => step.stage_type === "review");
  if (!review) throw new Error("the Office workflow has no review step");
  await apiClient.moveTask(taskId, seed.workflowId, review.id);
}

/**
 * Publishes a PR head with checks (and optional changed files) and links it to
 * the task, which triggers the CI verifier.
 */
export async function linkPullRequestHead(
  apiClient: ApiClient,
  seed: Seed,
  taskId: string,
  pr: { head: string; checks: MockCheckRun[]; files?: string[] },
): Promise<void> {
  const { head, checks, files = [] } = pr;
  await apiClient.mockGitHubSetWorkspaceConnection(seed.workspaceId, {
    source: "legacy_shared",
    status: "active",
  });
  await apiClient.mockGitHubAddPRs([
    {
      number: GATE_PR,
      title: "Gated change",
      state: "open",
      head_branch: "feature/gated",
      head_sha: head,
      base_branch: "main",
      author_login: "worker",
      repo_owner: GATE_OWNER,
      repo_name: GATE_REPO,
    },
  ]);
  await apiClient.mockGitHubAddCheckRuns(GATE_OWNER, GATE_REPO, head, checks);
  if (files.length) {
    await apiClient.mockGitHubAddPRFiles(
      GATE_OWNER,
      GATE_REPO,
      GATE_PR,
      files.map((filename) => ({ filename, status: "modified", additions: 1, deletions: 0 })),
    );
  }
  await apiClient.mockGitHubAssociateTaskPR({
    task_id: taskId,
    workspace_id: seed.workspaceId,
    owner: GATE_OWNER,
    repo: GATE_REPO,
    pr_number: GATE_PR,
    pr_url: `https://github.com/${GATE_OWNER}/${GATE_REPO}/pull/${GATE_PR}`,
    pr_title: "Gated change",
    head_branch: "feature/gated",
    base_branch: "main",
    author_login: "worker",
    head_sha: head,
    state: "open",
  });
}

export function completedCheck(name: string, conclusion: "success" | "failure"): MockCheckRun {
  return { name, status: "completed", conclusion };
}

/** A stored criterion's result or hold kind, read from the backend. */
export async function criterionState(
  apiClient: ApiClient,
  taskId: string,
  criterionId: string,
): Promise<string> {
  const response = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}/completion-gate`);
  if (!response.ok) return `http ${response.status}`;
  const gate = (await response.json()) as {
    criteria: Array<{ id: string; result?: string; hold_kind?: string }>;
  };
  const criterion = gate.criteria.find((item) => item.id === criterionId);
  return criterion?.hold_kind ? `held:${criterion.hold_kind}` : (criterion?.result ?? "missing");
}

/** A stored criterion's result reason, read from the backend. */
export async function criterionReason(
  apiClient: ApiClient,
  taskId: string,
  criterionId: string,
): Promise<string> {
  const response = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}/completion-gate`);
  if (!response.ok) return `http ${response.status}`;
  const gate = (await response.json()) as {
    criteria: Array<{ id: string; result_reason?: string }>;
  };
  return gate.criteria.find((item) => item.id === criterionId)?.result_reason ?? "missing";
}

/** The stored check criterion's result or hold kind. */
export function ciCriterionState(apiClient: ApiClient, taskId: string): Promise<string> {
  return criterionState(apiClient, taskId, "github_check");
}
