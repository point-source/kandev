import { describe, expect, it } from "vitest";
import type { TaskCompletionCriterion } from "@/lib/api/domains/task-completion-gates-api";
import {
  completionGateRefreshKey,
  completingStepId,
  criterionActions,
  criterionDisplayStatus,
  isProtectedPathHold,
  overriddenBy,
  shortHeads,
} from "./criteria-display";

function criterion(fields: Partial<TaskCompletionCriterion>): TaskCompletionCriterion {
  return {
    id: "github_check",
    description: "CI",
    evidence_subject: { kind: "github_pr_heads", id: "t1" },
    criterion_revision: 1,
    ...fields,
  };
}

describe("criterionDisplayStatus", () => {
  it("prefers a hold, then the blocker kind, then the result", () => {
    expect(criterionDisplayStatus(criterion({ hold_kind: "rework_limit_reached" }), "held")).toBe(
      "needs_you",
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
  it("shortens every head of a revision", () => {
    expect(shortHeads("pr-1@a1b2c3d4e5,pr-2@0123456789")).toEqual(["a1b2c3d", "0123456"]);
    expect(shortHeads(undefined)).toEqual([]);
  });

  it("classifies holds", () => {
    expect(isProtectedPathHold(criterion({ hold_kind: "protected_files_changed" }))).toBe(true);
    expect(isProtectedPathHold(criterion({ hold_kind: "verification_limit_reached" }))).toBe(false);
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

describe("criterionActions", () => {
  const check = (fields: Partial<TaskCompletionCriterion> = {}) =>
    criterion({ kind: "check", ...fields });
  const review = (fields: Partial<TaskCompletionCriterion> = {}) =>
    criterion({ id: "review", kind: "review", ...fields });

  it("offers each hold kind its own actions", () => {
    expect(criterionActions(check({ hold_kind: "protected_files_changed" }), "task")).toEqual([
      "allow",
      "send_back",
    ]);
    expect(criterionActions(check({ hold_kind: "rework_limit_reached" }), "task")).toEqual([
      "extend_rounds",
    ]);
    expect(criterionActions(check({ hold_kind: "verification_limit_reached" }), "inbox")).toEqual([
      "extend_attempts",
    ]);
    expect(criterionActions(review({ hold_kind: "rework_limit_reached" }), "inbox")).toEqual([
      "extend_rounds",
      "accept",
    ]);
    expect(criterionActions(review({ hold_kind: "no_reviewer_available" }), "inbox")).toEqual([
      "accept",
    ]);
  });

  it("lets a person send a review back from the task page at any time", () => {
    expect(criterionActions(review({ result: "pass" }), "task")).toEqual(["send_back"]);
    expect(criterionActions(review({ hold_kind: "review_wait_limit_reached" }), "task")).toEqual([
      "accept",
      "send_back",
    ]);
    expect(criterionActions(review({ result: "pass" }), "inbox")).toEqual([]);
    expect(criterionActions(check({ result: "fail" }), "task")).toEqual([]);
  });
});
