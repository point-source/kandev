import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
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

// i18n-exempt: test id.
const TEXTAREA = "office-events-textarea";

afterEach(cleanup);

/** Applies updates to the step, as the workflow card does. */
function Harness({
  initial,
  others,
  onUpdate,
}: {
  initial: WorkflowStep;
  others: WorkflowStep[];
  onUpdate: (updates: Partial<WorkflowStep>) => void;
}) {
  const [current, setCurrent] = useState(initial);
  return (
    <OfficeStepControls
      step={current}
      steps={[current, ...others]}
      onUpdate={(updates) => {
        onUpdate(updates);
        setCurrent((previous) => ({ ...previous, ...updates }));
      }}
      readOnly={false}
    />
  );
}

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
    const textarea = screen.getByTestId(TEXTAREA) as HTMLTextAreaElement;
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
      office_events_draft: undefined,
    });
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-003.3
  it("keeps invalid YAML as a draft with a located error instead of changing events", () => {
    const onUpdate = vi.fn();
    render(<Harness initial={review} others={[steps[1]!]} onUpdate={onUpdate} />);
    const invalid = "on_enter:\n  - type: ensure_seat\n";
    fireEvent.change(screen.getByTestId(TEXTAREA), { target: { value: invalid } });
    expect(onUpdate).toHaveBeenLastCalledWith({ office_events_draft: invalid });
    expect(onUpdate).not.toHaveBeenCalledWith(
      expect.objectContaining({ events: expect.anything() }),
    );
    expect(screen.getByTestId("office-events-error").textContent).toBe(
      i18n.t("workflows:officeEventsErrorUnknownAction", {
        trigger: "on_enter",
        action: "ensure_seat",
      }),
    );
    expect((screen.getByTestId(TEXTAREA) as HTMLTextAreaElement).value).toBe(invalid);
  });

  it("follows a rename of a referenced step", () => {
    const { rerender } = render(
      <OfficeStepControls step={review} steps={steps} onUpdate={vi.fn()} readOnly={false} />,
    );
    const renamed = [review, step("s-done", "Finished")];
    rerender(
      <OfficeStepControls step={review} steps={renamed} onUpdate={vi.fn()} readOnly={false} />,
    );
    const textarea = screen.getByTestId(TEXTAREA) as HTMLTextAreaElement;
    expect(textarea.value).toContain("step_id: Finished");
    expect(screen.queryByTestId("office-events-error")).toBeNull();
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-003.1, AC-OFFICE-WORKFLOW-SELECT-003.4
  it("disables editing when read-only", () => {
    render(<OfficeStepControls step={review} steps={steps} onUpdate={vi.fn()} readOnly />);
    expect(screen.getByTestId(TEXTAREA).hasAttribute("disabled")).toBe(true);
    expect(screen.getByTestId("step-stage-type-select").hasAttribute("disabled")).toBe(true);
  });

  it("opens events as a plain disclosure without a nested frame", () => {
    render(<OfficeStepControls step={review} steps={steps} onUpdate={vi.fn()} readOnly={false} />);
    const disclosure = screen.getByTestId("office-events-editor");
    expect(disclosure.className).not.toMatch(/\bborder\b/);
  });
});
