import { expect } from "@playwright/test";
import { test } from "../../fixtures/office-fixture";
import { assertNoDocumentHorizontalOverflow } from "../../helpers/layout-assertions";
import { WorkflowSettingsPage } from "../../pages/workflow-settings-page";
import { chooseOfficeWorkflow, selectOfficeWorkflowApi } from "./office-workflow-selection-helpers";

test.describe("mobile: Office workflow selection", () => {
  test.afterEach(async ({ apiClient, officeSeed }) => {
    await selectOfficeWorkflowApi(apiClient, officeSeed.workspaceId, officeSeed.workflowId);
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-002.6, AC-OFFICE-WORKFLOW-SELECT-004.6
  test("shows the section, picker, refusal and duplicate notice without overflow", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    await testPage.setViewportSize({ width: 390, height: 844 });
    const { workspaceId, workflowId: builtInId } = officeSeed;
    await apiClient.createWorkflow(workspaceId, "Empty mobile candidate");

    const settings = new WorkflowSettingsPage(testPage);
    await settings.goto(workspaceId);
    await expect(testPage.getByTestId("office-workflows-section")).toBeVisible();
    await expect(testPage.getByTestId("office-workflow-select")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);

    await chooseOfficeWorkflow(testPage, "Empty mobile candidate");
    await expect(testPage.getByTestId("office-workflow-refusal")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);

    await settings.duplicateWorkflow(settings.workflowCard(builtInId), true);
    await expect(testPage.getByTestId("office-builtin-copy-notice")).toBeVisible();
    await assertNoDocumentHorizontalOverflow(testPage);
  });
});
