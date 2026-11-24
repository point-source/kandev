import { describe, expect, it } from "vitest";
import {
  describeOfficeWorkflowProblem,
  describeOfficeWorkflowProblems,
  type Translate,
} from "./problem-messages";

const t: Translate = (key, values) => (values ? `${key} ${JSON.stringify(values)}` : key);

describe("describeOfficeWorkflowProblem", () => {
  it("names the step, role and action for structural problems", () => {
    const text = describeOfficeWorkflowProblem(t, {
      code: "missing_seat_action",
      step_name: "Verify",
      role: "reviewer",
      action: "ensure_participant_seat",
    });
    expect(text).toContain("workflows:officeProblemMissingSeatAction");
    expect(text).toContain('"step":"Verify"');
    expect(text).toContain('"role":"reviewer"');
  });

  it("passes the start step count for plural copy", () => {
    expect(describeOfficeWorkflowProblem(t, { code: "start_step_count", count: 2 })).toContain(
      '"count":2',
    );
  });

  it("maps refusal codes and falls back for unknown codes", () => {
    expect(describeOfficeWorkflowProblem(t, { code: "holds_kanban_tasks" })).toBe(
      "workflows:officeProblemHoldsKanbanTasks",
    );
    expect(describeOfficeWorkflowProblem(t, { code: "brand_new" })).toContain(
      "workflows:officeProblemUnknown",
    );
  });

  it("lists every problem", () => {
    const text = describeOfficeWorkflowProblems(t, [
      { code: "missing_work_stage" },
      { code: "sync_managed" },
    ]);
    expect(text.split("\n")).toEqual([
      "workflows:officeProblemMissingWorkStage",
      "workflows:officeProblemSyncManaged",
    ]);
  });
});
