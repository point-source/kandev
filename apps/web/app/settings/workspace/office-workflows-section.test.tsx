import { cleanup, render, screen, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { getOfficeWorkflowState } from "@/lib/api/domains/office-workflow-api";
import { i18n } from "@/lib/i18n";
import { workflowId, workspaceId, type Workflow, type Workspace } from "@/lib/types/http";
import { OfficeWorkflowsSection } from "./office-workflows-section";

vi.mock("@/lib/api/domains/office-workflow-api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api/domains/office-workflow-api")>();
  return { ...actual, getOfficeWorkflowState: vi.fn(), selectOfficeWorkflow: vi.fn() };
});
vi.mock("@/lib/routing/client-router", () => ({ useRouter: () => ({ refresh: vi.fn() }) }));
vi.mock("./workspace-workflows-list", () => ({
  WorkflowList: ({
    workflowItems,
    renderNotice,
  }: {
    workflowItems: Workflow[];
    renderNotice: (workflow: Workflow) => React.ReactNode;
  }) => (
    <div>
      {workflowItems.map((workflow) => (
        <div key={workflow.id} data-testid={`card-${workflow.id}`}>
          {renderNotice(workflow)}
          {workflow.name}
        </div>
      ))}
    </div>
  ),
}));

const workspace = {
  id: workspaceId("ws-1"),
  name: "Office",
  office_workflow_id: workflowId("wf-copy"),
} as Workspace;

const builtIn = {
  id: workflowId("wf-builtin"),
  workspace_id: workspace.id,
  name: "Office Default",
  is_office: true,
  is_system: true,
  created_at: "",
  updated_at: "",
} as Workflow;
const copy = { ...builtIn, id: workflowId("wf-copy"), name: "Lean office", is_system: false };

beforeEach(() => {
  vi.mocked(getOfficeWorkflowState).mockResolvedValue({
    selected_workflow_id: "wf-copy",
    candidates: [
      { id: "wf-builtin", name: "Office Default", built_in: true, is_office: true },
      { id: "wf-copy", name: "Lean office", built_in: false, is_office: true },
    ],
  });
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
});

describe("OfficeWorkflowsSection", () => {
  // @covers AC-OFFICE-WORKFLOW-SELECT-002.1
  it("lists the built-in and copies with Built-in and Selected badges", async () => {
    render(
      <OfficeWorkflowsSection workspace={workspace} officeWorkflows={[builtIn, copy]} canManage />,
    );
    expect(screen.getByTestId("office-workflows-section")).toBeTruthy();
    expect(screen.getByTestId("office-workflow-badges-wf-builtin").textContent).toContain(
      i18n.t("workflows:officeBuiltInBadge"),
    );
    expect(screen.getByTestId("office-workflow-badges-wf-copy").textContent).toContain(
      i18n.t("workflows:officeSelectedBadge"),
    );
    await waitFor(() => expect(getOfficeWorkflowState).toHaveBeenCalledWith("ws-1"));
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-004.5
  it("disables the picker for users who cannot manage the workspace", async () => {
    render(
      <OfficeWorkflowsSection
        workspace={workspace}
        officeWorkflows={[builtIn, copy]}
        canManage={false}
      />,
    );
    await waitFor(() => expect(getOfficeWorkflowState).toHaveBeenCalled());
    expect(screen.getByTestId("office-workflow-select").hasAttribute("disabled")).toBe(true);
    expect(screen.getByText(i18n.t("workflows:officeWorkflowPickerNoAccess"))).toBeTruthy();
  });
});
