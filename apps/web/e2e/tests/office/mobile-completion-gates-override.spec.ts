import { test, expect } from "../../fixtures/office-fixture";
import { seedGatedTask } from "../../helpers/completion-gates";

/** Phone parity for REQ-OFFICE-COMPLETION-GATES-008.3. */
test.describe("Office completion override on mobile", () => {
  test("overrides from a bottom sheet with a reason", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(apiClient, officeSeed, "Mobile gate override");
    await testPage.goto(`/office/tasks/${taskId}`);
    const section = testPage.getByTestId("completion-criteria-section");
    await section.getByTestId("completion-override").click();
    const sheet = testPage.getByTestId("completion-dialog-override");
    await sheet.getByTestId("completion-action-reason").fill("Checked on device");
    await sheet.getByTestId("completion-action-confirm").click();
    await expect.poll(async () => (await apiClient.getTask(taskId)).state).toBe("COMPLETED");
  });
});
