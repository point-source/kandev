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
      { kind: "ci_at_head", protected_paths: ["tests/**", ".github/**"], max_rework_rounds: 2 },
      { kind: "review_at_head" },
    ]);
    expect(selection).toMatchObject({
      ci: true,
      review: true,
      protectedPaths: "tests/**, .github/**",
      maxRework: "2",
    });
    expect(criteriaFromSelection(selection)).toEqual([
      { kind: "ci_at_head", max_rework_rounds: 2, protected_paths: ["tests/**", ".github/**"] },
      { kind: "review_at_head", max_rework_rounds: 2 },
    ]);
  });

  it("sends nothing for an empty selection and drops blank values", () => {
    expect(criteriaFromSelection(EMPTY_SELECTION)).toEqual([]);
    expect(
      criteriaFromSelection({
        ...EMPTY_SELECTION,
        ci: true,
        protectedPaths: " , ",
        maxRework: "x",
      }),
    ).toEqual([{ kind: "ci_at_head" }]);
  });

  it("validates limits against ceilings", () => {
    const ok = {
      default_max_attempts: 3,
      default_max_rework: 3,
      ceiling_attempts: 10,
      ceiling_rework: 10,
    };
    expect(invalidLimits(ok)).toBe(false);
    expect(invalidLimits({ ...ok, default_max_rework: 11 })).toBe(true);
    expect(invalidLimits({ ...ok, ceiling_attempts: 0 })).toBe(true);
  });

  it("maps refusal codes to copy keys", () => {
    expect(completionCriteriaErrorKey("completion_criteria_requires_repository")).toBe(
      "office:completionGatesErrRequiresRepository",
    );
    expect(completionCriteriaErrorKey("something else")).toBeUndefined();
  });
});
