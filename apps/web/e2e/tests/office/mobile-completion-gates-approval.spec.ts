import { test, expect } from "../../fixtures/office-fixture";
import {
  completedCheck,
  criterionReason,
  linkPullRequestHead,
  moveToReviewStep,
  seedGatedTask,
} from "../../helpers/completion-gates";

/** Phone parity for REQ-OFFICE-COMPLETION-GATES-010: approving from the Inbox. */
test.describe("Office completion gate human approval on mobile", () => {
  test("approves from the Inbox in a bottom sheet", async ({ testPage, apiClient, officeSeed }) => {
    const taskId = await seedGatedTask(apiClient, officeSeed, "Mobile approval", [], ["approval"]);
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "e5e5505",
      checks: [completedCheck("unit", "success")],
    });
    await moveToReviewStep(apiClient, officeSeed, taskId);
    await expect.poll(() => criterionReason(apiClient, taskId, "approval")).toBe("awaiting_person");

    await testPage.goto("/office/inbox");
    const item = testPage.getByTestId("inbox-item-completion_gate_approval");
    await expect(item.getByTestId("inbox-completion-hold")).toHaveText("Approval needed");
    for (const testId of ["inbox-completion-approve", "inbox-completion-send_back"]) {
      const box = await item.getByTestId(testId).boundingBox();
      expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    }
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(overflow).toBe(false);

    await item.getByTestId("inbox-completion-approve").click();
    const sheet = testPage.getByTestId("completion-dialog-approve");
    await sheet.getByTestId("completion-action-confirm").click();
    await expect(item).toHaveCount(0);

    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-approval");
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Passed");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Approved by a person·e5e5505",
    );
  });
});
