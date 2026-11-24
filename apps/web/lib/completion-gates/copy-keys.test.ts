import { describe, expect, it } from "vitest";
import { ApiError } from "@/lib/api/client";
import { actionErrorKey } from "./copy-keys";

describe("actionErrorKey", () => {
  it("names a criterion that changed since the dialog opened", () => {
    const stale = new ApiError("conflict", 409, { code: "task_completion_evidence_changed" });
    expect(actionErrorKey(stale)).toBe("office:completionGatesActionStale");
  });

  it("falls back to the generic failure", () => {
    expect(
      actionErrorKey(new ApiError("conflict", 409, { code: "completion_limit_above_ceiling" })),
    ).toBe("office:completionGatesActionFailed");
    expect(actionErrorKey(new Error("network"))).toBe("office:completionGatesActionFailed");
  });
});
