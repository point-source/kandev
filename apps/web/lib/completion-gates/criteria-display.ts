import type {
  TaskCompletionCriterion,
  TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";

export type CriterionDisplayStatus =
  | "pending"
  | "pass"
  | "fail"
  | "cannot_evaluate"
  | "held"
  | "stale";

const PROTECTED_HOLD = "protected_path_changed";
const CAP_HOLDS = new Set(["rework_cap_reached", "attempt_cap_reached"]);

/** The blocker reason of a criterion, or undefined when it is met. */
export function criterionBlocker(
  gate: TaskCompletionGateSnapshot,
  criterionId: string,
): string | undefined {
  return gate.blockers?.find((blocker) => blocker.criterion_id === criterionId)?.reason;
}

/** The single status a person sees for one criterion. */
export function criterionDisplayStatus(
  criterion: TaskCompletionCriterion,
  blocker: string | undefined,
): CriterionDisplayStatus {
  if (criterion.held_reason) return "held";
  if (!blocker) return "pass";
  if (blocker === "evidence_stale") return "stale";
  if (criterion.result === "fail") return "fail";
  if (criterion.result === "cannot_evaluate") return "cannot_evaluate";
  return "pending";
}

/** The reason code shown under a criterion: hold, result, or blocker. */
export function criterionReason(
  criterion: TaskCompletionCriterion,
  blocker: string | undefined,
): string | undefined {
  if (criterion.held_reason) return criterion.held_reason;
  if (criterion.result_reason) return criterion.result_reason;
  return blocker;
}

/** Short commit SHAs of a PR heads revision ("pr@sha,pr@sha"). */
export function shortHeads(revision: string | undefined): string[] {
  if (!revision) return [];
  return revision
    .split(",")
    .map((entry) => entry.split("@")[1] ?? "")
    .filter(Boolean)
    .map((sha) => sha.slice(0, 7));
}

export function isProtectedPathHold(criterion: TaskCompletionCriterion): boolean {
  return criterion.held_reason === PROTECTED_HOLD;
}

export function isCapHold(criterion: TaskCompletionCriterion): boolean {
  return CAP_HOLDS.has(criterion.held_reason ?? "");
}

type StepLike = { id: string; position: number; complete_task_on_enter?: boolean };

/** The workflow's completing step: the final step, when it completes tasks. */
export function completingStepId(steps: StepLike[]): string | undefined {
  const final = steps.reduce<StepLike | undefined>(
    (best, step) => (!best || step.position > best.position ? step : best),
    undefined,
  );
  return final?.complete_task_on_enter ? final.id : undefined;
}

type HistoryLike = { action: string; actor_id: string; created_at: string };

/** The person who overrode the last completion, if any. */
export function overriddenBy(history: HistoryLike[]): string | undefined {
  const overrides = history.filter((entry) => entry.action === "completion_overridden");
  return overrides.at(-1)?.actor_id;
}

type GateSummaryLike = { revision: number; state_revision?: number } | null | undefined;

/**
 * Identifies the gate state shown for a task: the criteria revision plus the
 * state revision bumped by every result, hold or control change, and the task
 * status (Done changes the header).
 */
export function completionGateRefreshKey(status: string, summary: GateSummaryLike): string {
  return `${status}|${summary?.revision ?? 0}|${summary?.state_revision ?? 0}`;
}
