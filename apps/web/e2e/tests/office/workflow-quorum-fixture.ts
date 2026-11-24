import { test as base, expect } from "../../fixtures/office-fixture";
import { createOfficeWorkflowCopy } from "../../helpers/office-workflow-copy";

export const test = base.extend<{ quorumWorkflowId: string }>({
  quorumWorkflowId: async ({ apiClient, officeSeed }, use) => {
    // Quorum decisions use a seeded idle session. Returning to Work must not
    // launch that session while the test verifies the next manual transition,
    // so the spec runs on an Office copy whose Work step does not auto-start.
    const copy = await createOfficeWorkflowCopy(
      apiClient,
      officeSeed.workspaceId,
      officeSeed.workflowId,
      (steps) => {
        const work = steps.find((step) => step.stage_type === "work");
        expect(work).toBeDefined();
        const events = work!.events ?? {};
        expect(events.on_enter?.some((action) => action.type === "auto_start_agent")).toBe(true);
        work!.events = {
          ...events,
          on_enter: events.on_enter?.filter((action) => action.type !== "auto_start_agent"),
        };
      },
    );
    await use(copy.workflowId);
  },
});

export { expect };
