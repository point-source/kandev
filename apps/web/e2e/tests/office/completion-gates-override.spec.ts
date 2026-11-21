import { test, expect } from "../../fixtures/office-fixture";
import { seedGatedTask } from "../../helpers/completion-gates";

/**
 * REQ-OFFICE-COMPLETION-GATES-008.3: a person overrides a blocked completion
 * with a recorded reason; tasks without criteria show no section.
 */
test.describe("Office completion gate: override", () => {
  test("overrides a blocked completion with a reason", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(apiClient, officeSeed, "Gate override flow");
    await testPage.goto(`/office/tasks/${taskId}`);
    const section = testPage.getByTestId("completion-criteria-section");
    await expect(section.getByTestId("completion-criterion-status")).toHaveText("Pending");

    await section.getByTestId("completion-override").click();
    const dialog = testPage.getByTestId("completion-dialog-override");
    await expect(dialog.getByTestId("completion-action-confirm")).toBeDisabled();
    await dialog.getByTestId("completion-action-reason").fill("Verified by hand on staging");
    await dialog.getByTestId("completion-action-confirm").click();

    await expect.poll(async () => (await apiClient.getTask(taskId)).state).toBe("COMPLETED");
    await expect(section.getByTestId("completion-criteria-summary")).toContainText(
      "Done by override",
    );
  });

  test("a task without criteria shows no section and reads no gate", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const task = await apiClient.createTask(officeSeed.workspaceId, "Ungated task", {
      workflow_id: officeSeed.workflowId,
    });
    const gateReads: string[] = [];
    testPage.on("request", (request) => {
      if (request.url().includes("/completion-gate")) gateReads.push(request.url());
    });
    await testPage.goto(`/office/tasks/${task.id}`);
    await expect(testPage.getByTestId("office-new-issue-dialog")).toHaveCount(0);
    await expect(testPage.getByText("Ungated task").first()).toBeVisible();
    await expect(testPage.getByTestId("completion-criteria-section")).toHaveCount(0);
    expect(gateReads).toEqual([]);
  });
});
