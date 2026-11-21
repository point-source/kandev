import { test, expect } from "../../fixtures/office-fixture";
import { completedCheck, linkPullRequestHead, seedGatedTask } from "../../helpers/completion-gates";

/** Phone parity for REQ-OFFICE-COMPLETION-GATES-003/008: the protected-path hold on a phone. */
test.describe("Office completion gate on mobile", () => {
  test("shows the hold and allows it from a bottom sheet", async ({
    testPage,
    apiClient,
    officeSeed,
  }) => {
    const taskId = await seedGatedTask(apiClient, officeSeed, "Mobile gate protected", [
      "tests/**",
    ]);
    await linkPullRequestHead(apiClient, officeSeed, taskId, {
      head: "dddd444",
      checks: [completedCheck("unit", "success")],
      files: ["tests/very/long/path/to/a/protected/webhook_integration_test.go"],
    });

    await testPage.goto(`/office/tasks/${taskId}`);
    const row = testPage.getByTestId("completion-criterion-github_check");
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Needs you");
    const allow = row.getByTestId("completion-allow");
    const box = await allow.boundingBox();
    expect(box?.height ?? 0).toBeGreaterThanOrEqual(44);
    const overflow = await testPage.evaluate(
      () => document.documentElement.scrollWidth > window.innerWidth,
    );
    expect(overflow).toBe(false);

    await allow.click();
    await testPage
      .getByTestId("completion-dialog-allow")
      .getByTestId("completion-action-confirm")
      .click();
    await expect(row.getByTestId("completion-criterion-status")).toHaveText("Passed");
  });
});
