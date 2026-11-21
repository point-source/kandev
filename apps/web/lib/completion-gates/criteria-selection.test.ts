import { describe, expect, it } from "vitest";
import {
  completionCriteriaErrorKey,
  criteriaFromSelection,
  EMPTY_SELECTION,
  invalidLimits,
  selectionFromCriteria,
} from "./criteria-selection";

describe("criteria selection", () => {
  it("round-trips a workspace default", () => {
    const selection = selectionFromCriteria([
      {
        kind: "check",
        runner: "github",
        protected_paths: ["tests/**", ".github/**"],
        rework_limit: 2,
      },
      { kind: "review", review_wait_limit: 90 },
    ]);
    expect(selection).toMatchObject({
      ci: true,
      review: true,
      protectedPaths: "tests/**, .github/**",
      reworkLimit: "2",
      reviewWaitLimit: "90",
    });
    expect(criteriaFromSelection(selection)).toEqual([
      {
        kind: "check",
        runner: "github",
        rework_limit: 2,
        protected_paths: ["tests/**", ".github/**"],
      },
      { kind: "review", rework_limit: 2, review_wait_limit: 90 },
    ]);
  });

  it("sends nothing for an empty selection and drops blank values", () => {
    expect(criteriaFromSelection(EMPTY_SELECTION)).toEqual([]);
    expect(
      criteriaFromSelection({
        ...EMPTY_SELECTION,
        ci: true,
        protectedPaths: " , ",
        reworkLimit: "x",
        reviewWaitLimit: "30",
      }),
    ).toEqual([{ kind: "check", runner: "github" }]);
  });

  it("validates limits against ceilings", () => {
    const ok = {
      default_verification_limit: 3,
      default_rework_limit: 3,
      verification_limit_ceiling: 10,
      rework_limit_ceiling: 10,
    };
    expect(invalidLimits(ok)).toBe(false);
    expect(invalidLimits({ ...ok, default_rework_limit: 11 })).toBe(true);
    expect(invalidLimits({ ...ok, verification_limit_ceiling: 0 })).toBe(true);
  });

  it("maps refusal codes to copy keys", () => {
    expect(completionCriteriaErrorKey("completion_criteria_requires_repository")).toBe(
      "office:completionGatesErrRequiresRepository",
    );
    expect(completionCriteriaErrorKey("something else")).toBeUndefined();
  });
});
