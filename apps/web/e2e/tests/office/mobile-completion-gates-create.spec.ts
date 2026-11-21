import { test } from "../../fixtures/office-fixture";
import {
  createWithCriteriaWithoutRepository,
  saveGateDefaults,
} from "../../helpers/completion-gates-create";

/** Phone parity for REQ-OFFICE-COMPLETION-GATES-001/008.7/008.8. */
test.describe("Office completion gate settings and creation on mobile", () => {
  test("defaults prefill the dialog and a repository-less project is refused", async ({
    testPage,
    officeApi,
    officeSeed,
  }) => {
    await saveGateDefaults(testPage);
    await createWithCriteriaWithoutRepository(testPage, officeApi, officeSeed.workspaceId, true);
  });
});
