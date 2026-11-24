import { test, expect } from "../../fixtures/office-fixture";
import {
  completedCheck,
  criterionReason,
  linkPullRequestHead,
  moveToReviewStep,
  seedGatedTask,
} from "../../helpers/completion-gates";

/**
 * REQ-OFFICE-COMPLETION-GATES-010: a human approval asks a person only after
 * the task leaves the work stage and the other criteria pass; the person
 * approves or sends the task back at the commit shown.
 */
test.describe("Office completion gate: human approval", () => {
  test("asks after the check passes and the task leaves Work, then a person approves in the Inbox", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(
      apiClient,
      officeSeed,
      "Gate approval approve",
      [],
      ["check", "approval"],
    );
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "a1a1101",
      checks: [completedCheck("unit", "success")],
    });
    await expect
      .poll(() => criterionReason(apiClient, taskId, "approval"))
      .toBe("awaiting_criteria");

    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-approval");
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Pending");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Asked after the task leaves the work stage·a1a1101",
    );
    await expect(row.getByTestId("completion-criterion-counts")).toHaveText("Rework rounds 0 of 3");

    await moveToReviewStep(apiClient, officeSeed, taskId);
    await expect.poll(() => criterionReason(apiClient, taskId, "approval")).toBe("awaiting_person");
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Needs you");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Waiting for your approval·a1a1101",
    );

    await testPage.goto("/office/inbox");
    await expect(testPage.getByTestId("inbox-item-task_review_request")).toHaveCount(0);
    const item = testPage.getByTestId("inbox-item-completion_gate_approval");
    await expect(item.getByTestId("inbox-completion-hold")).toHaveText("Approval needed");
    await expect(item.getByTestId("inbox-completion-commits")).toHaveText("a1a1101");
    await item.getByTestId("inbox-completion-approve").click();
    const dialog = testPage.getByTestId("completion-dialog-approve");
    await dialog.getByTestId("completion-action-reason").fill("Checked the redirect flow");
    await dialog.getByTestId("completion-action-confirm").click();
    await expect(item).toHaveCount(0);

    await testPage.goto(`/office/tasks/${taskId}`);
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Passed");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Approved by a person: Checked the redirect flow·a1a1101",
    );
  });

  test("sends the task back from the task page with a reason", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(
      apiClient,
      officeSeed,
      "Gate approval send back",
      [],
      ["approval"],
    );
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "b2b2202",
      checks: [completedCheck("unit", "success")],
    });
    await moveToReviewStep(apiClient, officeSeed, taskId);
    await expect.poll(() => criterionReason(apiClient, taskId, "approval")).toBe("awaiting_person");

    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-approval");
    await row.getByTestId("completion-send-back").click();
    const dialog = testPage.getByTestId("completion-dialog-send_back");
    await dialog.getByTestId("completion-action-reason").fill("The redirect still loops");
    await dialog.getByTestId("completion-action-confirm").click();

    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Failed");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Sent back: The redirect still loops·b2b2202",
    );
    await expect(row.getByTestId("completion-criterion-counts")).toHaveText("Rework rounds 1 of 3");
  });

  test("refuses an approval after a push moved the heads", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(
      apiClient,
      officeSeed,
      "Gate approval stale",
      [],
      ["approval"],
    );
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "c3c3303",
      checks: [completedCheck("unit", "success")],
    });
    await moveToReviewStep(apiClient, officeSeed, taskId);
    await expect.poll(() => criterionReason(apiClient, taskId, "approval")).toBe("awaiting_person");

    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-approval");
    await row.getByTestId("completion-approve").click();
    const dialog = testPage.getByTestId("completion-dialog-approve");
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "d4d4404",
      checks: [completedCheck("unit", "success")],
    });
    await dialog.getByTestId("completion-action-confirm").click();
    await expect(dialog.getByRole("alert")).toHaveText(
      "This criterion changed since you opened it, so nothing was applied. Close this and check the criterion again.",
    );
  });
});
