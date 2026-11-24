import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  createOfficeWorkflow,
  OfficeWorkflowRefusedError,
  saveOfficeWorkflowDefinition,
} from "@/lib/api/domains/office-workflow-api";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import { persistOfficeWorkflowDraft } from "./office-workflow-persistence";

vi.mock("@/lib/api/domains/office-workflow-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/office-workflow-api")>();
  return { ...actual, createOfficeWorkflow: vi.fn(), saveOfficeWorkflowDefinition: vi.fn() };
});

const workflow = (id: string): Workflow =>
  ({
    id,
    workspace_id: "ws-1",
    name: " Office copy ",
    is_office: true,
    created_at: "",
    updated_at: "",
  }) as Workflow;

const step = (id: string, position: number): WorkflowStep =>
  ({
    id,
    workflow_id: "wf",
    name: id,
    position,
    color: "bg-blue-500",
    created_at: "",
    updated_at: "",
  }) as WorkflowStep;

beforeEach(() => vi.clearAllMocks());

describe("persistOfficeWorkflowDraft", () => {
  it("creates a new Office copy in one request and maps draft step ids", async () => {
    vi.mocked(createOfficeWorkflow).mockResolvedValue({
      workflow_id: "wf-new",
      workspace_id: "ws-1",
      updated_at: "2026-11-24T00:00:00Z",
      steps: [step("s-1", 0), step("s-2", 1)],
    });
    const progress = { stepIds: new Map<string, string>() };
    const result = await persistOfficeWorkflowDraft({
      workflow: workflow("temp-workflow-1"),
      draftSteps: [step("temp-step-a", 0), step("temp-step-b", 1)],
      progress,
    });

    const [workspaceId, body] = vi.mocked(createOfficeWorkflow).mock.calls[0]!;
    expect(workspaceId).toBe("ws-1");
    expect(body.name).toBe("Office copy");
    expect(body.steps[0]).not.toHaveProperty("created_at");
    expect(result.workflow).toMatchObject({ id: "wf-new", is_office: true, style: "office" });
    expect(progress.stepIds.get("temp-step-b")).toBe("s-2");
    expect(saveOfficeWorkflowDefinition).not.toHaveBeenCalled();
  });

  it("replaces a persisted Office copy", async () => {
    vi.mocked(saveOfficeWorkflowDefinition).mockResolvedValue({
      workflow_id: "wf-1",
      workspace_id: "ws-1",
      updated_at: "x",
      steps: [step("s-1", 0)],
    });
    await persistOfficeWorkflowDraft({
      workflow: workflow("wf-1"),
      draftSteps: [step("s-1", 0)],
      progress: { stepIds: new Map() },
    });
    expect(vi.mocked(saveOfficeWorkflowDefinition).mock.calls[0]![0]).toBe("wf-1");
  });

  it("turns a refusal into an error that lists every problem", async () => {
    vi.mocked(saveOfficeWorkflowDefinition).mockRejectedValue(
      new OfficeWorkflowRefusedError("refused", [
        { code: "missing_work_stage" },
        { code: "step_holds_tasks", step_name: "Done" },
      ]),
    );
    await expect(
      persistOfficeWorkflowDraft({
        workflow: workflow("wf-1"),
        draftSteps: [],
        progress: { stepIds: new Map() },
      }),
    ).rejects.toThrow(/\n/);
  });
});
