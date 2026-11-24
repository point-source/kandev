import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "../../helpers/api-client";

export type OfficeWorkflowState = {
  selected_workflow_id: string;
  candidates: { id: string; name: string; built_in: boolean; is_office: boolean }[];
};

export async function getOfficeWorkflowState(
  apiClient: ApiClient,
  workspaceId: string,
): Promise<OfficeWorkflowState> {
  const res = await apiClient.rawRequest(
    "GET",
    `/api/v1/workspaces/${workspaceId}/office-workflow`,
  );
  expect(res.ok).toBe(true);
  return (await res.json()) as OfficeWorkflowState;
}

export async function selectOfficeWorkflowApi(
  apiClient: ApiClient,
  workspaceId: string,
  workflowId: string,
): Promise<void> {
  const res = await apiClient.rawRequest(
    "PUT",
    `/api/v1/workspaces/${workspaceId}/office-workflow`,
    {
      workflow_id: workflowId,
    },
  );
  expect(res.ok, await res.clone().text()).toBe(true);
}

export async function candidateId(
  apiClient: ApiClient,
  workspaceId: string,
  name: string,
): Promise<string> {
  let id = "";
  await expect
    .poll(async () => {
      const state = await getOfficeWorkflowState(apiClient, workspaceId);
      id = state.candidates.find((candidate) => candidate.name === name)?.id ?? "";
      return id;
    })
    .not.toBe("");
  return id;
}

/** Creates an Office project; a task linked to it is an Office task. */
export async function createOfficeProject(apiClient: ApiClient, workspaceId: string, name: string) {
  const res = await apiClient.rawRequest(
    "POST",
    `/api/v1/office/workspaces/${workspaceId}/projects`,
    {
      name,
    },
  );
  expect(res.ok, await res.clone().text()).toBe(true);
  const body = (await res.json()) as { project?: { id?: string }; id?: string };
  return (body.project?.id ?? body.id) as string;
}

export async function createOfficeTask(
  apiClient: ApiClient,
  workspaceId: string,
  projectId: string,
  title: string,
): Promise<{ id: string; workflow_id: string }> {
  const res = await apiClient.rawRequest("POST", "/api/v1/tasks", {
    workspace_id: workspaceId,
    title,
    description: "",
    project_id: projectId,
  });
  expect(res.ok, await res.clone().text()).toBe(true);
  const body = (await res.json()) as {
    task?: { id: string; workflow_id: string };
    id?: string;
    workflow_id?: string;
  };
  return (body.task ?? body) as { id: string; workflow_id: string };
}

export async function getTask(
  apiClient: ApiClient,
  taskId: string,
): Promise<{ workflow_id: string; is_from_office?: boolean }> {
  const res = await apiClient.rawRequest("GET", `/api/v1/tasks/${taskId}`);
  expect(res.ok).toBe(true);
  return (await res.json()) as { workflow_id: string; is_from_office?: boolean };
}

/** Chooses an option in the Office workflow picker. */
export async function chooseOfficeWorkflow(page: Page, optionName: string): Promise<void> {
  await page.getByTestId("office-workflow-select").click();
  await page.getByRole("option", { name: optionName, exact: true }).click();
}
