import { test, expect } from "../../fixtures/office-fixture";
import {
  completedCheck,
  criterionState,
  linkPullRequestHead,
  moveToReviewStep,
  seedGatedTask,
} from "../../helpers/completion-gates";

/**
 * REQ-OFFICE-COMPLETION-GATES-004/008/009: a person sends a review back at the
 * commit shown, and a review no reviewer can approve holds and is accepted by
 * a person from the Inbox.
 */
test.describe("Office completion gate: review at the PR head", () => {
  test("sends a review back at the commit shown", async ({ testPage, apiClient, officeSeed }) => {
    const taskId = await seedGatedTask(
      apiClient,
      officeSeed,
      "Gate review send back",
      [],
      ["review"],
    );
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "ffff666",
      checks: [completedCheck("unit", "success")],
    });

    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-review");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Needs approval·ffff666",
    );
    await expect(testPage.getByTestId("completion-auth-warning")).toBeVisible();
    await row.getByTestId("completion-send-back").click();
    const dialog = testPage.getByTestId("completion-dialog-send_back");
    await dialog.getByTestId("completion-action-reason").fill("Cover the empty input");
    await dialog.getByTestId("completion-action-confirm").click();

    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Failed");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Sent back: Cover the empty input·ffff666",
    );
    await expect(row.getByTestId("completion-criterion-counts")).toContainText("Rework rounds 1");
  });

  test("holds a review no reviewer can approve, then a person accepts it in the Inbox", async ({
    testPage,
    apiClient,
    officeApi,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(
      apiClient,
      officeSeed,
      "Gate review no reviewer",
      [],
      ["review"],
    );
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "abab777",
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
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "No reviewer available: the only reviewer is the task's assignee·abab777",
    );

    await testPage.goto("/office/inbox");
    const item = testPage.getByTestId("inbox-item-completion_gate_hold");
    await expect(item.getByTestId("inbox-completion-hold")).toHaveText(
      "No reviewer available: the only reviewer is the task's assignee",
    );
    await item.getByTestId("inbox-completion-accept").click();
    const dialog = testPage.getByTestId("completion-dialog-accept");
    await dialog.getByTestId("completion-action-reason").fill("Read the diff myself");
    await dialog.getByTestId("completion-action-confirm").click();
    await expect(item).toHaveCount(0);

    await testPage.goto(`/office/tasks/${taskId}`);
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Passed");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Accepted by a person: Read the diff myself·abab777",
    );
  });
});
