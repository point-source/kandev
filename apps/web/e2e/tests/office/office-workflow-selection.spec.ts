import { expect } from "@playwright/test";
import { test } from "../../fixtures/office-fixture";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";
import {
  candidateId,
  chooseOfficeWorkflow,
  createOfficeProject,
  createOfficeTask,
  getOfficeWorkflowState,
  getTask,
  selectOfficeWorkflowApi,
} from "./office-workflow-selection-helpers";

const COPY_NAME = "Office Default (copy)";

test.describe("Office workflow selection", () => {
  test.afterEach(async ({ apiClient, officeSeed }) => {
    // Later tests in this worker expect the built-in Office workflow.
    await selectOfficeWorkflowApi(apiClient, officeSeed.workspaceId, officeSeed.workflowId);
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-001.1, AC-OFFICE-WORKFLOW-SELECT-001.2,
  // AC-OFFICE-WORKFLOW-SELECT-002.2, AC-OFFICE-WORKFLOW-SELECT-002.3,
  // AC-OFFICE-WORKFLOW-SELECT-003.1, AC-OFFICE-WORKFLOW-SELECT-003.2,
  // AC-OFFICE-WORKFLOW-SELECT-004.2
  test("duplicates the built-in, selects the copy, and starts new Office tasks on it", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const { workspaceId, workflowId: builtInId } = officeSeed;
    const projectId = await createOfficeProject(apiClient, workspaceId, "Office workflow switch");
    const oldTask = await createOfficeTask(apiClient, workspaceId, projectId, "Before switch");
    expect(oldTask.workflow_id).toBe(builtInId);

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(workspaceId);
    await expect(testPage.getByTestId("office-workflows-section")).toBeVisible();
    await expect(testPage.getByTestId(`office-workflow-badges-${builtInId}`)).toContainText(
      "Built-in",
    );

    await settings.duplicateWorkflow(settings.workflowCard(builtInId));
    const copyCard = await settings.findWorkflowCard(COPY_NAME, { waitForName: true });
    await expect(testPage.getByTestId("office-builtin-copy-notice")).toBeVisible();

    await settings.stepNodeByName(copyCard, "Review").first().click();
    await expect(copyCard.getByTestId("step-stage-type-select")).toBeVisible();
    const events = copyCard.getByTestId("office-events-editor");
    await events.locator("summary").click();
    await expect(events.getByTestId("office-events-textarea")).toHaveValue(
      /ensure_participant_seat/,
    );

    // The built-in reject loop (Review back to Work) is a confirmed cycle.
    await settings.submitSaveChanges();
    await expect(settings.cycleGuardDialog).toBeVisible();
    await settings.cycleGuardDialog.getByRole("button", { name: "Create anyway" }).click();
    await expect(settings.cycleGuardDialog).not.toBeVisible();
    const copyId = await candidateId(apiClient, workspaceId, COPY_NAME);

    await chooseOfficeWorkflow(testPage, COPY_NAME);
    await expect(testPage.getByTestId(`office-workflow-badges-${copyId}`)).toContainText(
      "Selected",
    );
    expect((await getOfficeWorkflowState(apiClient, workspaceId)).selected_workflow_id).toBe(
      copyId,
    );

    const newTask = await createOfficeTask(apiClient, workspaceId, projectId, "After switch");
    expect(newTask.workflow_id).toBe(copyId);
    const reloadedOld = await getTask(apiClient, oldTask.id);
    expect(reloadedOld.workflow_id).toBe(builtInId);
    expect(reloadedOld.is_from_office).toBe(true);

    const kanban = await apiClient.listWorkflows(workspaceId);
    const kanbanIds = kanban.workflows.map((workflow) => workflow.id);
    expect(kanbanIds).not.toContain(copyId);
    expect(kanbanIds).not.toContain(builtInId);
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-004.4, AC-OFFICE-WORKFLOW-SELECT-005.6,
  // AC-OFFICE-WORKFLOW-SELECT-006.4
  test("refuses a workflow Office cannot drive and keeps the selection", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const { workspaceId, workflowId: builtInId } = officeSeed;
    await apiClient.createWorkflow(workspaceId, "Empty Office candidate");

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(workspaceId);
    await chooseOfficeWorkflow(testPage, "Empty Office candidate");
    const refusal = testPage.getByTestId("office-workflow-refusal");
    await expect(refusal).toBeVisible();
    await expect(refusal.locator("li")).toHaveCount(3);
    expect((await getOfficeWorkflowState(apiClient, workspaceId)).selected_workflow_id).toBe(
      builtInId,
    );

    const builtInSteps = await apiClient.listWorkflowSteps(builtInId);
    const res = await apiClient.rawRequest(
      "DELETE",
      `/api/v1/workflow/steps/${builtInSteps.steps[0]!.id}`,
    );
    expect(res.status).toBe(409);
    expect(await res.text()).toContain("built_in_read_only");
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-007.1, AC-OFFICE-WORKFLOW-SELECT-007.2
  test("exports the Office workflow and imports a selectable Office copy", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const { workspaceId, workflowId: builtInId } = officeSeed;
    const yaml = await apiClient.exportWorkflow(builtInId);
    expect(yaml).toContain("office: true");
    expect(yaml).toContain("stage_type: review");
    const imported = await apiClient.importWorkflows(
      workspaceId,
      yaml.replace("name: Office Default", "name: Imported Office"),
    );
    expect(imported.created).toEqual(["Imported Office"]);
    const importedId = await candidateId(apiClient, workspaceId, "Imported Office");

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(workspaceId);
    const section = testPage.getByTestId("office-workflows-section");
    await expect(section.getByTestId(`workflow-card-${importedId}`)).toBeVisible();
    await chooseOfficeWorkflow(testPage, "Imported Office");
    await expect(testPage.getByTestId(`office-workflow-badges-${importedId}`)).toContainText(
      "Selected",
    );
  });
});
