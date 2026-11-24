import { describe, expect, it } from "vitest";
import { ApiError } from "../client";
import { OfficeWorkflowRefusedError, toOfficeWorkflowError } from "./office-workflow-api";

describe("toOfficeWorkflowError", () => {
  it("turns a refused write into an error carrying the problems", () => {
    const converted = toOfficeWorkflowError(
      new ApiError("refused", 422, {
        code: "office_workflow_refused",
        problems: [{ code: "missing_work_stage" }],
      }),
    );
    expect(converted).toBeInstanceOf(OfficeWorkflowRefusedError);
    expect((converted as OfficeWorkflowRefusedError).problems).toEqual([
      { code: "missing_work_stage" },
    ]);
  });

  it("leaves other errors untouched", () => {
    const other = new ApiError("boom", 500, { error: "boom" });
    expect(toOfficeWorkflowError(other)).toBe(other);
    const plain = new Error("x");
    expect(toOfficeWorkflowError(plain)).toBe(plain);
  });
});
