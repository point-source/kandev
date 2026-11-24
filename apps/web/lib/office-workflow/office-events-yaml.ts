import { parseDocument, stringify } from "yaml";
import type { StepEvents } from "@/lib/types/workflow-actions";
import type { WorkflowStep } from "@/lib/types/http";

// i18n-exempt: wire-format trigger and action names, not copy.
const MOVE_ACTIONS = ["move_to_next", "move_to_previous", "move_to_step"];
// i18n-exempt: wire-format action names, not copy.
const GENERIC_ACTIONS = [
  ...MOVE_ACTIONS,
  "auto_start_agent",
  "queue_run",
  "clear_decisions",
  "queue_run_for_each_participant",
];

/** Supported actions per step trigger; mirrors the backend's known actions. */
// i18n-exempt: wire-format trigger and action names, not copy.
const ACTIONS_BY_TRIGGER: Record<string, readonly string[]> = {
  on_enter: [
    "configure_session",
    "enable_plan_mode",
    "auto_start_agent",
    "reset_agent_context",
    "set_session_mode",
    "clear_decisions",
    "queue_run_for_each_participant",
    "queue_run",
    "ensure_participant_seat",
    "run_code_review",
  ],
  on_turn_start: MOVE_ACTIONS,
  on_turn_complete: [...MOVE_ACTIONS, "disable_plan_mode"],
  on_exit: ["disable_plan_mode"],
  on_comment: GENERIC_ACTIONS,
  on_blocker_resolved: GENERIC_ACTIONS,
  on_children_completed: GENERIC_ACTIONS,
  on_approval_resolved: GENERIC_ACTIONS,
  on_heartbeat: GENERIC_ACTIONS,
  on_budget_alert: GENERIC_ACTIONS,
  on_agent_error: GENERIC_ACTIONS,
  on_pr_merged: GENERIC_ACTIONS,
  on_pr_closed: GENERIC_ACTIONS,
};

const STEP_REFERENCE_KEY = "step_id";

export type OfficeEventsError =
  | { code: "syntax"; line: number; message: string }
  | { code: "notMapping" }
  | { code: "unknownTrigger"; trigger: string }
  | { code: "notList"; trigger: string }
  | { code: "badAction"; trigger: string; index: number }
  | { code: "unknownAction"; trigger: string; action: string }
  | { code: "unknownStep"; trigger: string; step: string }
  | { code: "duplicateStepNames"; step: string };

export type OfficeEventsParseResult =
  | { ok: true; events: StepEvents }
  | { ok: false; error: OfficeEventsError };

type Mapping = Record<string, unknown>;

function isMapping(value: unknown): value is Mapping {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function mapStepReferences(value: unknown, lookup: (ref: string) => string | undefined): unknown {
  if (Array.isArray(value)) return value.map((item) => mapStepReferences(item, lookup));
  if (!isMapping(value)) return value;
  return Object.fromEntries(
    Object.entries(value).map(([key, item]) => [
      key,
      key === STEP_REFERENCE_KEY && typeof item === "string"
        ? (lookup(item) ?? item)
        : mapStepReferences(item, lookup),
    ]),
  );
}

/** Serializes step events as YAML, naming referenced steps by step name. */
export function officeEventsToYaml(events: StepEvents | undefined, steps: WorkflowStep[]): string {
  const populated = Object.fromEntries(
    Object.entries(events ?? {}).filter(
      ([, actions]) => Array.isArray(actions) && actions.length > 0,
    ),
  );
  if (Object.keys(populated).length === 0) return "";
  const names = new Map(steps.map((step) => [step.id, step.name]));
  return stringify(mapStepReferences(populated, (id) => names.get(id)));
}

function duplicateStepName(steps: WorkflowStep[]): string | undefined {
  const seen = new Set<string>();
  for (const step of steps) {
    if (seen.has(step.name)) return step.name;
    seen.add(step.name);
  }
  return undefined;
}

function unresolvedStepName(value: unknown, ids: Map<string, string>): string | undefined {
  if (Array.isArray(value)) {
    for (const item of value) {
      const missing = unresolvedStepName(item, ids);
      if (missing) return missing;
    }
    return undefined;
  }
  if (!isMapping(value)) return undefined;
  for (const [key, item] of Object.entries(value)) {
    if (key === STEP_REFERENCE_KEY && typeof item === "string" && !ids.has(item)) return item;
    const missing = unresolvedStepName(item, ids);
    if (missing) return missing;
  }
  return undefined;
}

function validateTrigger(
  trigger: string,
  actions: unknown,
  ids: Map<string, string>,
): OfficeEventsError | null {
  const allowed = ACTIONS_BY_TRIGGER[trigger];
  if (!allowed) return { code: "unknownTrigger", trigger };
  if (!Array.isArray(actions)) return { code: "notList", trigger };
  for (const [index, action] of actions.entries()) {
    if (!isMapping(action) || typeof action.type !== "string")
      return { code: "badAction", trigger, index };
    if (action.config !== undefined && !isMapping(action.config))
      return { code: "badAction", trigger, index };
    if (!allowed.includes(action.type))
      return { code: "unknownAction", trigger, action: action.type };
  }
  const missing = unresolvedStepName(actions, ids);
  return missing ? { code: "unknownStep", trigger, step: missing } : null;
}

/**
 * Parses Office events YAML. Step references use step names and are mapped
 * back to step IDs; unknown triggers, actions and steps are refused.
 */
export function parseOfficeEventsYaml(
  text: string,
  steps: WorkflowStep[],
): OfficeEventsParseResult {
  const duplicate = duplicateStepName(steps);
  if (duplicate !== undefined)
    return { ok: false, error: { code: "duplicateStepNames", step: duplicate } };
  const document = parseDocument(text);
  const syntaxError = document.errors[0];
  if (syntaxError) {
    const line = syntaxError.linePos?.[0]?.line ?? 1;
    return {
      ok: false,
      error: { code: "syntax", line, message: syntaxError.message.split("\n")[0] ?? "" },
    };
  }
  const parsed: unknown = document.toJS() ?? {};
  if (!isMapping(parsed)) return { ok: false, error: { code: "notMapping" } };
  const ids = new Map(steps.map((step) => [step.name, step.id]));
  for (const [trigger, actions] of Object.entries(parsed)) {
    const error = validateTrigger(trigger, actions, ids);
    if (error) return { ok: false, error };
  }
  return { ok: true, events: mapStepReferences(parsed, (name) => ids.get(name)) as StepEvents };
}
