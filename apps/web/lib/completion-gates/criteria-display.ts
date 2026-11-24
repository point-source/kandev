import type {
  TaskCompletionCriterion,
  TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";

export type CriterionDisplayStatus =
  | "pending"
  | "pass"
  | "fail"
  | "cannot_evaluate"
  | "needs_you"
  | "stale";

/** What a person can do about one criterion. */
export type CriterionAction =
  | "allow"
  | "send_back"
  | "extend_rounds"
  | "extend_attempts"
  | "accept"
  | "approve";

/** Where a person acts: the task page offers every action, the Inbox only hold actions. */
export type ActionSurface = "task" | "inbox";

const PROTECTED_HOLD = "protected_files_changed";
const LIMIT_HOLD_ACTIONS: Record<string, CriterionAction> = {
  rework_limit_reached: "extend_rounds",
  verification_limit_reached: "extend_attempts",
};

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
  if (criterion.hold_kind || asksPerson(criterion)) return "needs_you";
  if (!blocker) return "pass";
  if (blocker === "evidence_stale") return "stale";
  if (criterion.result === "fail") return "fail";
  if (criterion.result === "cannot_evaluate") return "cannot_evaluate";
  return "pending";
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

const AWAITING_PERSON = "awaiting_person";

/** True for an approval that asks a person to approve the commit shown. */
export function asksPerson(criterion: TaskCompletionCriterion): boolean {
  return (
    criterion.kind === "approval" &&
    !criterion.hold_kind &&
    criterion.result_reason === AWAITING_PERSON
  );
}

export function isProtectedPathHold(criterion: TaskCompletionCriterion): boolean {
  return criterion.hold_kind === PROTECTED_HOLD;
}

/**
 * The actions a person can take on a criterion. A protected-file hold takes
 * Allow or Send back; a limit hold takes more rounds or attempts; any held
 * review can be accepted at the commit shown; an approval that asks the person
 * can be approved or sent back. On the task page a review or an approval can
 * also be sent back at any commit shown, because rejecting fails closed.
 */
export function criterionActions(
  criterion: TaskCompletionCriterion,
  surface: ActionSurface,
): CriterionAction[] {
  const isReview = criterion.kind === "review";
  if (isProtectedPathHold(criterion)) return ["allow", "send_back"];
  const actions: CriterionAction[] = [];
  const limit = LIMIT_HOLD_ACTIONS[criterion.hold_kind ?? ""];
  if (limit) actions.push(limit);
  if (isReview && criterion.hold_kind) actions.push("accept");
  if (asksPerson(criterion)) actions.push("approve", "send_back");
  else if (sentBackAnyTime(criterion) && criterion.result_subject_revision && surface === "task") {
    actions.push("send_back");
  }
  return actions;
}

function sentBackAnyTime(criterion: TaskCompletionCriterion): boolean {
  return criterion.kind === "review" || criterion.kind === "approval";
}

/**
 * The criterion as a person sees and acts on it. A criterion with no result is
 * shown at the live PR heads. A review whose verdict belongs to earlier heads
 * no longer judges the PR, so it shows as awaiting a verdict at the live heads;
 * a hold keeps the heads it holds at, and every other result its own heads.
 */
export function criterionAtShownCommit(
  criterion: TaskCompletionCriterion,
  headsRevision: string | undefined,
): TaskCompletionCriterion {
  if (!headsRevision) return criterion;
  if (!criterion.result_subject_revision) {
    return { ...criterion, result_subject_revision: headsRevision };
  }
  const staleReview =
    sentBackAnyTime(criterion) &&
    !criterion.hold_kind &&
    criterion.result_subject_revision !== headsRevision;
  if (!staleReview) return criterion;
  return {
    ...criterion,
    result: "pending",
    result_reason: undefined,
    result_detail: [],
    evidence: undefined,
    result_subject_revision: headsRevision,
  };
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
