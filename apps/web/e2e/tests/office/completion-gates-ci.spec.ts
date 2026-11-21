import { test, expect } from "../../fixtures/office-fixture";
import {
  ciCriterionState,
  completedCheck,
  linkPullRequestHead,
  seedGatedTask,
} from "../../helpers/completion-gates";

/**
 * REQ-OFFICE-COMPLETION-GATES-002/003/008: Kandev verifies CI at the PR head,
 * holds a protected-path change for a person, and the Office task page shows
 * and recovers both.
 */
test.describe("Office completion gate: CI at the PR head", () => {
  test("shows failing CI, then a pass at a new head", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(apiClient, officeSeed, "Gate CI flow");
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "aaaa111",
      checks: [completedCheck("unit", "failure")],
    });

    await expect.poll(() => ciCriterionState(apiClient, taskId)).toBe("fail");
    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-github_check");
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Failed");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Failing: unit·aaaa111",
    );

    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "bbbb222",
      checks: [completedCheck("unit", "success")],
    });
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Passed");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText("bbbb222");
  });

  test("holds a protected-path change until a person allows it", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(apiClient, officeSeed, "Gate protected flow", ["tests/**"]);
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "cccc333",
      checks: [completedCheck("unit", "success")],
      files: ["tests/webhook_test.go"],
    });

    await expect
      .poll(() => ciCriterionState(apiClient, taskId))
      .toBe("held:protected_files_changed");
    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-github_check");
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Needs you");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Protected file changed: tests/webhook_test.go·cccc333",
    );

    await testPage.goto("/office/inbox");
    await expect(testPage.getByTestId("inbox-item-completion_gate_hold")).toBeVisible();

    await testPage.goto(`/office/tasks/${taskId}`);
    await row.getByTestId("completion-allow").click();
    const dialog = testPage.getByTestId("completion-dialog-allow");
    await dialog.getByTestId("completion-action-confirm").click();
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Passed");

    await testPage.goto("/office/inbox");
    await expect(testPage.getByTestId("inbox-item-completion_gate_hold")).toHaveCount(0);
  });

  test("sends a protected change back to the worker with a reason", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(apiClient, officeSeed, "Gate send back flow", ["tests/**"]);
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "eeee555",
      checks: [completedCheck("unit", "success")],
      files: ["tests/webhook_test.go"],
    });
    await expect
      .poll(() => ciCriterionState(apiClient, taskId))
      .toBe("held:protected_files_changed");

    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-github_check");
    await row.getByTestId("completion-send-back").click();
    const dialog = testPage.getByTestId("completion-dialog-send_back");
    await dialog.getByTestId("completion-action-reason").fill("Keep the old assertions");
    await dialog.getByTestId("completion-action-confirm").click();

    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Failed");
    await expect(row.getByTestId("completion-criterion-detail")).toHaveText(
      "Sent back: Keep the old assertions·eeee555",
    );
    await expect(row.getByTestId("completion-criterion-counts")).toContainText("Rework rounds 1");
  });
});
