import { test, expect } from "../../fixtures/office-fixture";
import {
  completedCheck,
  criterionState,
  linkPullRequestHead,
  moveToReviewStep,
  seedGatedTask,
} from "../../helpers/completion-gates";

/** Phone parity for REQ-OFFICE-COMPLETION-GATES-008/009: accepting and sending back a review. */
test.describe("Office completion gate review on mobile", () => {
  test("accepts a review no reviewer can approve from a bottom sheet", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(apiClient, officeSeed, "Mobile review hold", [], ["review"]);
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "cdcd888",
      checks: [completedCheck("unit", "success")],
    });
    await officeApi.assignTask(taskId, officeSeed.agentId);
    await moveToReviewStep(apiClient, officeSeed, taskId);
    await expect
      .poll(() => criterionState(apiClient, taskId, "review"))
      .toBe("held:no_reviewer_available");

    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-review");
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Needs you");
    for (const testId of ["completion-accept", "completion-send-back"]) {
      const box = await row.getByTestId(testId).boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(overflow).toBe(false);

    await row.getByTestId("completion-accept").click();
    const sheet = testPage.getByTestId("completion-dialog-accept");
    await sheet.getByTestId("completion-action-reason").fill("Read the diff myself");
    await sheet.getByTestId("completion-action-confirm").click();
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Passed");
  });
});
