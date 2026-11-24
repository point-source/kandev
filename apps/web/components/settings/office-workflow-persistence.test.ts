import { beforeEach, describe, expect, it, vi } from "vitest";
import { ApiError } from "@/lib/api/client";
import {
  createOfficeWorkflow,
  getOfficeWorkflowState,
  OfficeWorkflowRefusedError,
  saveOfficeWorkflowDefinition,
} from "@/lib/api/domains/office-workflow-api";
import { t } from "@/lib/i18n";
import type { Workflow, WorkflowStep } from "@/lib/types/http";
import {
  ensureOfficeWorkflowDeletable,
  persistOfficeWorkflowDraft,
} from "./office-workflow-persistence";

vi.mock("@/lib/api/domains/office-workflow-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/office-workflow-api")>();
  return {
    ...actual,
    createOfficeWorkflow: vi.fn(),
    saveOfficeWorkflowDefinition: vi.fn(),
    getOfficeWorkflowState: vi.fn(),
  };
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

const step = (id: string, position: number, updatedAt = ""): WorkflowStep =>
  ({
    id,
    workflow_id: "wf",
    name: id,
    position,
    color: "bg-blue-500",
    created_at: "",
    updated_at: updatedAt,
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
      savedSteps: [],
      progress,
    });

    const [workspaceId, body] = vi.mocked(createOfficeWorkflow).mock.calls[0]!;
    expect(workspaceId).toBe("ws-1");
    expect(body.name).toBe("Office copy");
    expect(body.steps[0]).not.toHaveProperty("created_at");
    expect(body).not.toHaveProperty("expected_updated_at");
    expect(result.workflow).toMatchObject({ id: "wf-new", is_office: true, style: "office" });
    expect(progress.stepIds.get("temp-step-b")).toBe("s-2");
    expect(saveOfficeWorkflowDefinition).not.toHaveBeenCalled();
  });

  it("replaces a persisted Office copy, fenced by the loaded versions", async () => {
    vi.mocked(saveOfficeWorkflowDefinition).mockResolvedValue({
      workflow_id: "wf-1",
      workspace_id: "ws-1",
      updated_at: "x",
      steps: [step("s-1", 0)],
    });
    const draft = { ...step("s-1", 0, "v-draft"), office_events_draft: "on_enter: [" };
    await persistOfficeWorkflowDraft({
      workflow: { ...workflow("wf-1"), updated_at: "v-workflow" },
      draftSteps: [draft],
      savedSteps: [step("s-1", 0, "v-step"), step("s-2", 1, "v-step-2")],
      progress: { stepIds: new Map() },
    });
    const [workflowId, body] = vi.mocked(saveOfficeWorkflowDefinition).mock.calls[0]!;
    expect(workflowId).toBe("wf-1");
    expect(body.expected_updated_at).toBe("v-workflow");
    expect(body.expected_steps).toEqual({ "s-1": "v-step", "s-2": "v-step-2" });
    expect(body.steps[0]).not.toHaveProperty("office_events_draft");
  });

  it("explains a version conflict", async () => {
    vi.mocked(saveOfficeWorkflowDefinition).mockRejectedValue(
      new ApiError("changed", 409, { code: "workflow_changed" }),
    );
    await expect(
      persistOfficeWorkflowDraft({
        workflow: workflow("wf-1"),
        draftSteps: [],
        savedSteps: [],
        progress: { stepIds: new Map() },
      }),
    ).rejects.toThrow(t("workflows:officeWorkflowChangedElsewhere"));
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
        savedSteps: [],
        progress: { stepIds: new Map() },
      }),
    ).rejects.toThrow(/\n/);
  });
});

describe("ensureOfficeWorkflowDeletable", () => {
  it("refuses the selected Office workflow before any task moves", async () => {
    vi.mocked(getOfficeWorkflowState).mockResolvedValue({
      selected_workflow_id: "wf-1",
      candidates: [],
    });
    await expect(ensureOfficeWorkflowDeletable(workflow("wf-1"))).rejects.toThrow(
      t("workflows:officeProblemSelectedDelete"),
    );
    await expect(ensureOfficeWorkflowDeletable(workflow("wf-2"))).resolves.toBeUndefined();
  });

  it("does not look up Kanban workflows", async () => {
    await ensureOfficeWorkflowDeletable({ ...workflow("wf-k"), is_office: false });
    expect(getOfficeWorkflowState).not.toHaveBeenCalled();
  });
});
