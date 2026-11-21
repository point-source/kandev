import { fetchJson, type ApiRequestOptions } from "../client";

export type TaskCompletionEvidenceSubject = {
  kind: "task_revision" | "execution" | "artifact_revision" | "github_pr_head" | "github_pr_heads";
  id: string;
  revision?: string;
};

export type TaskCompletionEvidence = {
  subject: TaskCompletionEvidenceSubject;
  summary?: string;
  reference?: string;
};

export type TaskCompletionCriterionKind = "attested" | "check" | "review";

export type TaskCompletionResult = "pending" | "pass" | "fail" | "cannot_evaluate";

/** What produced a verified criterion's verdict. Derived by the backend. */
export type TaskCompletionVerdictSource = "program" | "person" | "agent";

/** The program that decides a check criterion. */
export type TaskCompletionCheckRunner = "github";

/** Why automation stopped on a criterion and is waiting for a person. */
export type TaskCompletionHoldKind =
  | "protected_files_changed"
  | "rework_limit_reached"
  | "verification_limit_reached"
  | "no_reviewer_available"
  | "review_wait_limit_reached";

export type TaskCompletionCriterionConfig = {
  runner?: TaskCompletionCheckRunner;
  protected_paths?: string[];
  verification_limit?: number;
  rework_limit?: number;
  /** Minutes a parked reviewer may wait before a person is asked. */
  review_wait_limit?: number;
  limits_confirmed_by?: string;
};

export type TaskCompletionCriterion = {
  id: string;
  description: string;
  evidence_subject: TaskCompletionEvidenceSubject;
  criterion_revision: number;
  verified_revision?: number;
  evidence?: TaskCompletionEvidence | null;
  verifier_kind?: string;
  verifier_id?: string;
  verified_at?: string | null;
  kind?: TaskCompletionCriterionKind;
  config?: TaskCompletionCriterionConfig;
  verdict_source?: TaskCompletionVerdictSource;
  owner_kind?: string;
  owner_id?: string;
  result?: TaskCompletionResult;
  result_reason?: string;
  result_detail?: string[];
  result_subject_revision?: string;
  result_first_seen_at?: string | null;
  result_at?: string | null;
  attempt_count?: number;
  rework_count?: number;
  hold_kind?: TaskCompletionHoldKind;
  held_at?: string | null;
  protected_allowed_revision?: string;
  protected_allowed_by?: string;
};

export type TaskCompletionBlocker = {
  criterion_id: string;
  reason: string;
};

export type TaskCompletionGateSnapshot = {
  task_id: string;
  workspace_id: string;
  revision: number;
  criteria: TaskCompletionCriterion[];
  blockers?: TaskCompletionBlocker[];
  blocked: boolean;
  /** Live PR heads ("pr@sha,...") of a task with host-verified criteria. */
  heads_revision?: string;
  /** Increases on every result, hold or control change. */
  state_revision?: number;
};

export type TaskCompletionGateHistory = {
  id: string;
  task_id: string;
  workspace_id: string;
  revision: number;
  action: string;
  actor_kind: string;
  actor_id: string;
  reason?: string;
  details?: string;
  created_at: string;
};

export type TaskCompletionCriterionInput = {
  id: string;
  description: string;
  evidence_subject: Pick<TaskCompletionEvidenceSubject, "kind" | "id">;
};

export type SetTaskCompletionCriteriaRequest = {
  expected_revision: number;
  criteria: TaskCompletionCriterionInput[];
  human_confirmation?: { expected_revision: number; reason: string };
};

export type VerifyTaskCompletionCriterionRequest = {
  expected_revision: number;
  evidence: TaskCompletionEvidence;
};

export function getTaskCompletionGate(taskId: string, options?: ApiRequestOptions) {
  return fetchJson<TaskCompletionGateSnapshot>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/completion-gate`,
    options,
  );
}

export async function listTaskCompletionGateHistory(taskId: string, options?: ApiRequestOptions) {
  const response = await fetchJson<{ items: TaskCompletionGateHistory[] }>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/completion-gate/history`,
    options,
  );
  return response.items ?? [];
}

export function setTaskCompletionCriteria(
  taskId: string,
  request: SetTaskCompletionCriteriaRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskCompletionGateSnapshot>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/completion-gate/criteria`,
    {
      ...options,
      init: { method: "PUT", body: JSON.stringify(request), ...(options?.init ?? {}) },
    },
  );
}

export function verifyTaskCompletionCriterion(
  taskId: string,
  criterionId: string,
  request: VerifyTaskCompletionCriterionRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskCompletionGateSnapshot>(
    `/api/v1/tasks/${encodeURIComponent(taskId)}/completion-gate/criteria/${encodeURIComponent(criterionId)}/evidence`,
    {
      ...options,
      init: { method: "PUT", body: JSON.stringify(request), ...(options?.init ?? {}) },
    },
  );
}

/** A person's decision on one criterion at the heads they saw. */
export type PersonCriterionAction = {
  expected_criterion_revision: number;
  expected_subject_revision: string;
  reason?: string;
};

export type ExtendCompletionCriterionRequest = {
  expected_criterion_revision: number;
  verification_limit?: number;
  rework_limit?: number;
  reason?: string;
};

const officeCriterionPath = (taskId: string, criterionId: string) =>
  `/api/v1/office/tasks/${encodeURIComponent(taskId)}/completion-gate/criteria/${encodeURIComponent(criterionId)}`;

function postPersonAction(route: string) {
  return (
    taskId: string,
    criterionId: string,
    request: PersonCriterionAction,
    options?: ApiRequestOptions,
  ) =>
    fetchJson<TaskCompletionGateSnapshot>(`${officeCriterionPath(taskId, criterionId)}/${route}`, {
      ...options,
      init: { ...options?.init, method: "POST", body: JSON.stringify(request) },
    });
}

/** Allows a check's protected file changes at the heads shown. */
export const allowProtectedFiles = postPersonAction("allow");

/** Fails a criterion at the heads shown and returns the task to its worker. */
export const sendBackCriterion = postPersonAction("send-back");

/** Accepts a held review at the heads shown as the person's verdict. */
export const acceptCriterion = postPersonAction("accept");

/** Raises a criterion's limits and releases a limit hold. */
export function extendCompletionCriterion(
  taskId: string,
  criterionId: string,
  request: ExtendCompletionCriterionRequest,
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskCompletionGateSnapshot>(
    `${officeCriterionPath(taskId, criterionId)}/extend`,
    {
      ...options,
      init: { ...options?.init, method: "POST", body: JSON.stringify(request) },
    },
  );
}

/** A host-verified criterion's resolved limits and how much of each is used. */
export type EffectiveCompletionLimits = {
  criterion_id: string;
  verification_limit: number;
  rework_limit: number;
  attempt_count: number;
  rework_count: number;
};

/** Reads the effective limits of a task's host-verified criteria. */
export async function getEffectiveCompletionLimits(taskId: string, options?: ApiRequestOptions) {
  const response = await fetchJson<{ limits: EffectiveCompletionLimits[] }>(
    `/api/v1/office/tasks/${encodeURIComponent(taskId)}/completion-gate/limits`,
    options,
  );
  return response.limits ?? [];
}

export type TaskCompletionCriterionRequest = {
  kind: "check" | "review";
  runner?: TaskCompletionCheckRunner;
  protected_paths?: string[];
  verification_limit?: number;
  rework_limit?: number;
  review_wait_limit?: number;
};

export type CompletionGateSettings = {
  workspace_id: string;
  default_criteria: TaskCompletionCriterionRequest[];
  default_verification_limit: number;
  default_rework_limit: number;
  verification_limit_ceiling: number;
  rework_limit_ceiling: number;
  /** Minutes; zero means a parked review waits indefinitely. */
  default_review_wait_limit: number;
};

const settingsPath = (workspaceId: string) =>
  `/api/v1/office/workspaces/${encodeURIComponent(workspaceId)}/completion-gates/settings`;

export function getCompletionGateSettings(workspaceId: string, options?: ApiRequestOptions) {
  return fetchJson<CompletionGateSettings>(settingsPath(workspaceId), options);
}

export function updateCompletionGateSettings(
  workspaceId: string,
  settings: Omit<CompletionGateSettings, "workspace_id">,
  options?: ApiRequestOptions,
) {
  return fetchJson<CompletionGateSettings>(settingsPath(workspaceId), {
    ...options,
    init: { ...options?.init, method: "PUT", body: JSON.stringify(settings) },
  });
}
