import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { deleteWorkflowAction } from "@/app/actions/workspaces";
import { ApiError } from "@/lib/api/client";
import { workflowId, workspaceId, type Workflow, type WorkflowStep } from "@/lib/types/http";
import { useOfficeWorkflowItems } from "./use-office-workflow-items";

vi.mock("@/app/actions/workspaces", () => ({ deleteWorkflowAction: vi.fn() }));

const builtIn: Workflow = {
  id: workflowId("wf-builtin"),
  workspace_id: workspaceId("ws-1"),
  name: "Office Default",
  is_office: true,
  is_system: true,
  created_at: "",
  updated_at: "",
};

const step: WorkflowStep = {
  id: "step-work",
  workflow_id: workflowId("wf-builtin"),
  name: "Work",
  position: 0,
  color: "bg-blue-500",
  created_at: "",
  updated_at: "",
};

beforeEach(() => vi.clearAllMocks());

describe("useOfficeWorkflowItems", () => {
  // @covers AC-OFFICE-WORKFLOW-SELECT-002.2, AC-OFFICE-WORKFLOW-SELECT-002.3
  it("adds an Office copy draft after its source and flags built-in copies", () => {
    const { result } = renderHook(() => useOfficeWorkflowItems([builtIn]));
    act(() => result.current.duplicate(builtIn, [step]));
    const [, draft] = result.current.items;
    expect(draft?.name).toBe("Office Default (copy)");
    expect(draft?.is_office).toBe(true);
    expect(result.current.builtInCopyIds.has(draft!.id)).toBe(true);
    expect(result.current.initialSteps.get(draft!.id)).toHaveLength(1);
  });

  it("removes an unsaved draft without a request", async () => {
    const { result } = renderHook(() => useOfficeWorkflowItems([builtIn]));
    act(() => result.current.duplicate(builtIn, [step]));
    const draftId = result.current.items[1]!.id;
    await act(async () => result.current.remove(draftId));
    expect(result.current.items).toHaveLength(1);
    expect(deleteWorkflowAction).not.toHaveBeenCalled();
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-002.5
  it("surfaces a refused delete as a named reason", async () => {
    vi.mocked(deleteWorkflowAction).mockRejectedValue(
      new ApiError("refused", 409, {
        code: "office_workflow_refused",
        problems: [{ code: "selected_delete" }],
      }),
    );
    const copy = { ...builtIn, id: workflowId("wf-copy"), is_system: false };
    const { result } = renderHook(() => useOfficeWorkflowItems([builtIn, copy]));
    await expect(result.current.remove("wf-copy")).rejects.toThrow();
    expect(result.current.items).toHaveLength(2);
  });
});
