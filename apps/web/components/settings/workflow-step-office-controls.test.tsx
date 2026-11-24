import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { i18n } from "@/lib/i18n";
import { workflowId, type WorkflowStep } from "@/lib/types/http";
import { OfficeStepControls } from "./workflow-step-office-controls";

const step = (id: string, name: string, extra: Partial<WorkflowStep> = {}): WorkflowStep => ({
  id,
  workflow_id: workflowId("wf"),
  name,
  position: 0,
  color: "bg-blue-500",
  created_at: "",
  updated_at: "",
  ...extra,
});

afterEach(cleanup);

describe("OfficeStepControls", () => {
  const review = step("s-review", "Review", {
    stage_type: "review",
    events: { on_turn_complete: [{ type: "move_to_step", config: { step_id: "s-done" } }] },
  });
  const steps = [review, step("s-done", "Done")];

  // @covers AC-OFFICE-WORKFLOW-SELECT-003.2
  it("shows events as YAML with step names and saves edits as step ids", () => {
    const onUpdate = vi.fn();
    render(<OfficeStepControls step={review} steps={steps} onUpdate={onUpdate} readOnly={false} />);
    const textarea = screen.getByTestId("office-events-textarea") as HTMLTextAreaElement;
    expect(textarea.value).toContain("step_id: Done");

    fireEvent.change(textarea, {
      target: {
        value:
          "on_enter:\n  - type: run_code_review\n    config:\n      agent_profile_id: p-1\n" +
          "on_turn_complete:\n  - type: move_to_step\n    config:\n      step_id: Done\n",
      },
    });
    expect(onUpdate).toHaveBeenLastCalledWith({
      events: {
        on_enter: [{ type: "run_code_review", config: { agent_profile_id: "p-1" } }],
        on_turn_complete: [{ type: "move_to_step", config: { step_id: "s-done" } }],
      },
    });
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-003.3
  it("shows a located error and does not update on an unknown action", () => {
    const onUpdate = vi.fn();
    render(<OfficeStepControls step={review} steps={steps} onUpdate={onUpdate} readOnly={false} />);
    fireEvent.change(screen.getByTestId("office-events-textarea"), {
      target: { value: "on_enter:\n  - type: ensure_seat\n" },
    });
    expect(onUpdate).not.toHaveBeenCalled();
    expect(screen.getByTestId("office-events-error").textContent).toBe(
      i18n.t("workflows:officeEventsErrorUnknownAction", {
        trigger: "on_enter",
        action: "ensure_seat",
      }),
    );
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-003.1, AC-OFFICE-WORKFLOW-SELECT-003.4
  it("disables editing when read-only", () => {
    render(<OfficeStepControls step={review} steps={steps} onUpdate={vi.fn()} readOnly />);
    expect(screen.getByTestId("office-events-textarea").hasAttribute("disabled")).toBe(true);
    expect(screen.getByTestId("step-stage-type-select").hasAttribute("disabled")).toBe(true);
  });
});
