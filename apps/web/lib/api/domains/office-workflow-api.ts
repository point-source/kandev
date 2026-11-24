import { ApiError, fetchJson } from "../client";
import type { WorkflowStep } from "@/lib/types/http";

/** One reason an Office workflow write was refused. */
export type OfficeWorkflowProblem = {
  code: string;
  step_name?: string;
  role?: string;
  action?: string;
  count?: number;
  detail?: string;
};

export type OfficeWorkflowCandidate = {
  id: string;
  name: string;
  built_in: boolean;
  is_office: boolean;
};

export type OfficeWorkflowState = {
  selected_workflow_id: string;
  candidates: OfficeWorkflowCandidate[];
};

export type OfficeWorkflowDefinitionBody = {
  name: string;
  description: string;
  prompt: string;
  agent_profile_id: string;
  steps: WorkflowStep[];
  /** The loaded workflow version; a newer stored version refuses the save. */
  expected_updated_at?: string;
  /** Every loaded step's version; any other stored step set refuses the save. */
  expected_steps?: Record<string, string>;
};

export type OfficeWorkflowSaveResult = {
  workflow_id: string;
  workspace_id: string;
  updated_at: string;
  steps: WorkflowStep[];
};

// i18n-exempt: wire error code returned by the backend.
const OFFICE_WORKFLOW_REFUSED = "office_workflow_refused";
// i18n-exempt: wire error code returned by the backend.
const WORKFLOW_CHANGED = "workflow_changed";

/** Reports a definition save refused because the workflow changed after it loaded. */
export function isWorkflowChangedError(error: unknown): boolean {
  if (!(error instanceof ApiError)) return false;
  const body = error.body as { code?: unknown } | null;
  return body?.code === WORKFLOW_CHANGED;
}

/** A refused Office workflow write and every reason the backend named. */
export class OfficeWorkflowRefusedError extends Error {
  readonly problems: OfficeWorkflowProblem[];

  constructor(message: string, problems: OfficeWorkflowProblem[]) {
    super(message);
    this.name = "OfficeWorkflowRefusedError";
    this.problems = problems;
  }
}

/** Converts a refused-write ApiError into an OfficeWorkflowRefusedError. */
export function toOfficeWorkflowError(error: unknown): unknown {
  if (!(error instanceof ApiError)) return error;
  const body = error.body as { code?: unknown; problems?: unknown } | null;
  if (body?.code !== OFFICE_WORKFLOW_REFUSED || !Array.isArray(body.problems)) return error;
  return new OfficeWorkflowRefusedError(error.message, body.problems as OfficeWorkflowProblem[]);
}

async function withOfficeErrors<T>(request: Promise<T>): Promise<T> {
  try {
    return await request;
  } catch (error) {
    throw toOfficeWorkflowError(error);
  }
}

export function getOfficeWorkflowState(workspaceId: string) {
  return fetchJson<OfficeWorkflowState>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/office-workflow`,
    { cache: "no-store" },
  );
}

export function selectOfficeWorkflow(workspaceId: string, workflowId: string) {
  return withOfficeErrors(
    fetchJson<OfficeWorkflowState>(
      `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/office-workflow`,
      { init: { method: "PUT", body: JSON.stringify({ workflow_id: workflowId }) } },
    ),
  );
}

export function createOfficeWorkflow(workspaceId: string, body: OfficeWorkflowDefinitionBody) {
  return withOfficeErrors(
    fetchJson<OfficeWorkflowSaveResult>(
      `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/office-workflows`,
      { init: { method: "POST", body: JSON.stringify(body) } },
    ),
  );
}

export function saveOfficeWorkflowDefinition(
  workflowId: string,
  body: OfficeWorkflowDefinitionBody,
) {
  return withOfficeErrors(
    fetchJson<OfficeWorkflowSaveResult>(
      `/api/v1/workflows/${encodeURIComponent(workflowId)}/definition`,
      { init: { method: "PUT", body: JSON.stringify(body) } },
    ),
  );
}
