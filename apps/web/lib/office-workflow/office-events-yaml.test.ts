import { describe, expect, it } from "vitest";
import { workflowId, type WorkflowStep } from "@/lib/types/http";
import { officeEventsToYaml, parseOfficeEventsYaml } from "./office-events-yaml";

const step = (id: string, name: string): WorkflowStep => ({
  id,
  workflow_id: workflowId("wf"),
  name,
  position: 0,
  color: "bg-blue-500",
  created_at: "",
  updated_at: "",
});

const steps = [step("s-work", "Work"), step("s-review", "Review")];

describe("office events YAML", () => {
  // @covers AC-OFFICE-WORKFLOW-SELECT-003.2
  it("round-trips events and shows step references as names", () => {
    const events = {
      on_enter: [
        { type: "ensure_participant_seat", config: { role: "reviewer" } },
        { type: "run_code_review", config: { agent_profile_id: "profile-1" } },
      ],
      on_turn_complete: [{ type: "move_to_step", config: { step_id: "s-review" } }],
      on_comment: [],
    } as const;
    const text = officeEventsToYaml(events as never, steps);
    expect(text).toContain("step_id: Review");
    expect(text).not.toContain("on_comment");
    const parsed = parseOfficeEventsYaml(text, steps);
    expect(parsed).toEqual({
      ok: true,
      events: {
        on_enter: events.on_enter,
        on_turn_complete: [{ type: "move_to_step", config: { step_id: "s-review" } }],
      },
    });
  });

  it("returns empty text and empty events for no events", () => {
    expect(officeEventsToYaml(undefined, steps)).toBe("");
    expect(parseOfficeEventsYaml("", steps)).toEqual({ ok: true, events: {} });
  });

  // @covers AC-OFFICE-WORKFLOW-SELECT-003.3
  it.each([
    ["on_enter: [", "syntax"],
    ["- a\n- b", "notMapping"],
    ["on_wake:\n  - type: queue_run", "unknownTrigger"],
    ["on_enter: queue_run", "notList"],
    ["on_enter:\n  - config: {}", "badAction"],
    ["on_enter:\n  - type: ensure_seat", "unknownAction"],
    ["on_turn_complete:\n  - type: move_to_step\n    config:\n      step_id: Ship", "unknownStep"],
  ])("refuses %j with %s", (text, code) => {
    const parsed = parseOfficeEventsYaml(text, steps);
    expect(parsed.ok).toBe(false);
    if (!parsed.ok) expect(parsed.error.code).toBe(code);
  });

  it("reports the line of a syntax error", () => {
    const parsed = parseOfficeEventsYaml("on_enter:\n  - type: x\n bad: [", steps);
    expect(parsed.ok).toBe(false);
    if (!parsed.ok && parsed.error.code === "syntax") expect(parsed.error.line).toBeGreaterThan(1);
  });

  it("refuses editing while step names are not unique", () => {
    const parsed = parseOfficeEventsYaml("", [step("a", "Same"), step("b", "Same")]);
    expect(parsed).toEqual({ ok: false, error: { code: "duplicateStepNames", step: "Same" } });
  });
});
