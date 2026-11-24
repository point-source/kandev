import { expect } from "@playwright/test";
import type { ApiClient } from "./api-client";

type Step = Awaited<ReturnType<ApiClient["listWorkflowSteps"]>>["steps"][number];

let copyCounter = 0;

/**
 * Creates an unselected Office copy of an Office workflow. The selected and
 * built-in Office workflows refuse single-step edits, so specs that need a
 * reconfigured Office workflow edit a copy instead. `mutate` changes the
 * copied steps before they are saved; step references are remapped.
 */
export async function createOfficeWorkflowCopy(
  apiClient: ApiClient,
  workspaceId: string,
  sourceWorkflowId: string,
  mutate: (steps: Step[]) => void = () => undefined,
): Promise<{ workflowId: string; steps: Step[] }> {
  const { steps } = await apiClient.listWorkflowSteps(sourceWorkflowId);
  const draft = structuredClone([...steps].sort((a, b) => a.position - b.position));
  mutate(draft);
  copyCounter += 1;
  const res = await apiClient.rawRequest(
    "POST",
    `/api/v1/workspaces/${workspaceId}/office-workflows`,
    {
      name: `E2E Office copy ${Date.now()}-${copyCounter}`,
      description: "",
      prompt: "",
      agent_profile_id: "",
      steps: draft.map(({ created_at: _created, updated_at: _updated, ...step }) => step),
    },
  );
  expect(res.ok, await res.clone().text()).toBe(true);
  const body = (await res.json()) as { workflow_id: string; steps: Step[] };
  return { workflowId: body.workflow_id, steps: body.steps };
}
