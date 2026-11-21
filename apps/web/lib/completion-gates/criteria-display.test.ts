import { describe, expect, it } from "vitest";
import type { TaskCompletionCriterion } from "@/lib/api/domains/task-completion-gates-api";
import {
  completionGateRefreshKey,
  completingStepId,
  criterionDisplayStatus,
  criterionReason,
  isCapHold,
  isProtectedPathHold,
  overriddenBy,
  shortHeads,
} from "./criteria-display";

function criterion(fields: Partial<TaskCompletionCriterion>): TaskCompletionCriterion {
  return {
    id: "ci_at_head",
    description: "CI",
    evidence_subject: { kind: "github_pr_heads", id: "t1" },
    criterion_revision: 1,
    ...fields,
  };
}

describe("criterionDisplayStatus", () => {
  it("prefers a hold, then the blocker kind, then the result", () => {
    expect(criterionDisplayStatus(criterion({ held_reason: "rework_cap_reached" }), "held")).toBe(
      "held",
    );
    expect(criterionDisplayStatus(criterion({ result: "pass" }), undefined)).toBe("pass");
    expect(criterionDisplayStatus(criterion({ result: "pass" }), "evidence_stale")).toBe("stale");
    expect(criterionDisplayStatus(criterion({ result: "fail" }), "unverified")).toBe("fail");
    expect(criterionDisplayStatus(criterion({ result: "cannot_evaluate" }), "unverified")).toBe(
      "cannot_evaluate",
    );
    expect(criterionDisplayStatus(criterion({}), "pr_missing")).toBe("pending");
  });
});

describe("criterion helpers", () => {
  it("chooses the most specific reason", () => {
    expect(criterionReason(criterion({ held_reason: "protected_path_changed" }), "held")).toBe(
      "protected_path_changed",
    );
    expect(criterionReason(criterion({ result_reason: "checks_failed" }), "unverified")).toBe(
      "checks_failed",
    );
    expect(criterionReason(criterion({}), "pr_missing")).toBe("pr_missing");
  });

  it("shortens every head of a revision", () => {
    expect(shortHeads("pr-1@a1b2c3d4e5,pr-2@0123456789")).toEqual(["a1b2c3d", "0123456"]);
    expect(shortHeads(undefined)).toEqual([]);
  });

  it("classifies holds", () => {
    expect(isProtectedPathHold(criterion({ held_reason: "protected_path_changed" }))).toBe(true);
    expect(isCapHold(criterion({ held_reason: "attempt_cap_reached" }))).toBe(true);
    expect(isCapHold(criterion({ held_reason: "protected_path_changed" }))).toBe(false);
  });

  it("finds the completing step and the overriding person", () => {
    expect(
      completingStepId([
        { id: "work", position: 1 },
        { id: "done", position: 4, complete_task_on_enter: true },
      ]),
    ).toBe("done");
    expect(completingStepId([{ id: "done", position: 4 }])).toBeUndefined();
    expect(
      overriddenBy([
        { action: "criteria_set", actor_id: "u0", created_at: "1" },
        { action: "completion_overridden", actor_id: "u1", created_at: "2" },
      ]),
    ).toBe("u1");
  });
});

describe("completionGateRefreshKey", () => {
  it("changes with the state revision even when counts do not", () => {
    const before = completionGateRefreshKey("in_progress", { revision: 1, state_revision: 4 });
    const after = completionGateRefreshKey("in_progress", { revision: 1, state_revision: 5 });
    expect(after).not.toBe(before);
    expect(completionGateRefreshKey("done", { revision: 1, state_revision: 5 })).not.toBe(after);
    expect(completionGateRefreshKey("todo", null)).toBe("todo|0|0");
  });
});
