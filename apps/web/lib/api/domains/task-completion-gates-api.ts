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

export type TaskCompletionCriterionKind = "attested" | "ci_at_head" | "review_at_head";

export type TaskCompletionResult = "pending" | "pass" | "fail" | "cannot_evaluate";

export type TaskCompletionConfidence = "machine" | "independent_check" | "judgement";

export type TaskCompletionCriterionConfig = {
  protected_paths?: string[];
  max_verification_attempts?: number;
  max_rework_rounds?: number;
  caps_confirmed_by?: string;
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
  confidence?: TaskCompletionConfidence;
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
  held_reason?: string;
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
  weakest_confidence?: TaskCompletionConfidence;
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

export type ProtectedPathsDecision = {
  action: "allow" | "send_back";
  expected_criterion_revision: number;
  expected_subject_revision: string;
  reason?: string;
};

export type ExtendCompletionCriterionRequest = {
  expected_criterion_revision: number;
  max_verification_attempts?: number;
  max_rework_rounds?: number;
  reason?: string;
};

const officeCriterionPath = (taskId: string, criterionId: string) =>
  `/api/v1/office/tasks/${encodeURIComponent(taskId)}/completion-gate/criteria/${encodeURIComponent(criterionId)}`;

/** A person's Allow or Send back decision on a protected-path hold. */
export function decideProtectedPaths(
  taskId: string,
  criterionId: string,
  request: ProtectedPathsDecision,
  options?: ApiRequestOptions,
) {
  return fetchJson<TaskCompletionGateSnapshot>(
    `${officeCriterionPath(taskId, criterionId)}/protected-paths`,
    { ...options, init: { ...options?.init, method: "POST", body: JSON.stringify(request) } },
  );
}

/** Raises a criterion's caps and releases a cap hold. */
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

/** A host-verified criterion's resolved caps and how much of each is used. */
export type EffectiveCompletionCaps = {
  criterion_id: string;
  max_verification_attempts: number;
  max_rework_rounds: number;
  attempt_count: number;
  rework_count: number;
};

/** Reads the effective caps of a task's host-verified criteria. */
export async function getEffectiveCompletionCaps(taskId: string, options?: ApiRequestOptions) {
  const response = await fetchJson<{ caps: EffectiveCompletionCaps[] }>(
    `/api/v1/office/tasks/${encodeURIComponent(taskId)}/completion-gate/caps`,
    options,
  );
  return response.caps ?? [];
}

export type TaskCompletionCriterionRequest = {
  kind: "ci_at_head" | "review_at_head";
  protected_paths?: string[];
  max_verification_attempts?: number;
  max_rework_rounds?: number;
};

export type CompletionGateSettings = {
  workspace_id: string;
  default_criteria: TaskCompletionCriterionRequest[];
  default_max_attempts: number;
  default_max_rework: number;
  ceiling_attempts: number;
  ceiling_rework: number;
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
