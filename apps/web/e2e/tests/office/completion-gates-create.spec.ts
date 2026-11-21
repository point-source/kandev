import { test } from "../../fixtures/office-fixture";
import {
  createWithCriteriaWithoutRepository,
  saveGateDefaults,
} from "../../helpers/completion-gates-create";

/**
 * REQ-OFFICE-COMPLETION-GATES-001/008.7/008.8: workspace defaults prefill the
 * new-task criteria group, the auth warning shows while review is selected,
 * and a refusal is shown as a clear message.
 */
test.describe("Office completion gate settings and creation", () => {
  test("defaults prefill the dialog and a repository-less project is refused", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await saveGateDefaults(testPage);
    await createWithCriteriaWithoutRepository(testPage, officeApi, officeSeed.workspaceId, false);
  });
});
