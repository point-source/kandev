import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  getOfficeWorkflowState,
  OfficeWorkflowRefusedError,
  selectOfficeWorkflow,
} from "@/lib/api/domains/office-workflow-api";
import { useOfficeWorkflowSelection } from "./use-office-workflow-selection";

vi.mock("@/lib/api/domains/office-workflow-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/office-workflow-api")>();
  return { ...actual, getOfficeWorkflowState: vi.fn(), selectOfficeWorkflow: vi.fn() };
});

const initial = {
  selected_workflow_id: "wf-builtin",
  candidates: [
    { id: "wf-builtin", name: "Office Default", built_in: true, is_office: true },
    { id: "wf-copy", name: "Copy", built_in: false, is_office: true },
  ],
};

beforeEach(() => {
  vi.clearAllMocks();
  vi.mocked(getOfficeWorkflowState).mockResolvedValue(initial);
});

describe("useOfficeWorkflowSelection", () => {
  it("loads the selection and candidates", async () => {
    const { result } = renderHook(() => useOfficeWorkflowSelection("ws-1"));
    await waitFor(() => expect(result.current.state).toEqual(initial));
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-004.2
  it("selects a candidate", async () => {
    vi.mocked(selectOfficeWorkflow).mockResolvedValue({
      ...initial,
      selected_workflow_id: "wf-copy",
    });
    const { result } = renderHook(() => useOfficeWorkflowSelection("ws-1"));
    await waitFor(() => expect(result.current.state).not.toBeNull());
    let outcome = "";
    await act(async () => {
      outcome = await result.current.select("wf-copy");
    });
    expect(outcome).toBe("selected");
    expect(result.current.state?.selected_workflow_id).toBe("wf-copy");
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-004.4, AC-OFFICE-WORKFLOW-SELECT-005.6
  it("keeps the previous selection and exposes every problem on refusal", async () => {
    vi.mocked(selectOfficeWorkflow).mockRejectedValue(
      new OfficeWorkflowRefusedError("refused", [
        { code: "missing_work_stage" },
        { code: "final_step_not_completing", step_name: "Ship" },
      ]),
    );
    const { result } = renderHook(() => useOfficeWorkflowSelection("ws-1"));
    await waitFor(() => expect(result.current.state).not.toBeNull());
    let outcome = "";
    await act(async () => {
      outcome = await result.current.select("wf-copy");
    });
    expect(outcome).toBe("refused");
    expect(result.current.state?.selected_workflow_id).toBe("wf-builtin");
    expect(result.current.problems.map((p) => p.code)).toEqual([
      "missing_work_stage",
      "final_step_not_completing",
    ]);
  });
});
