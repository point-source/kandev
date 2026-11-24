// Translation keys for completion-gate values. Keys, not copy: components
// resolve them with t() at render.
import { ApiError } from "@/lib/api/client";
import type { CriterionAction, CriterionDisplayStatus } from "./criteria-display";

/** Criterion names by kind; a check is named with its runner (github). */
export const KIND_KEYS: Record<string, string> = {
  check: "office:completionGatesKindCheckGithub",
  review: "office:completionGatesKindReview",
  approval: "office:completionGatesKindApproval",
  attested: "office:completionGatesKindAttested",
};

export const STATUS_KEYS: Record<CriterionDisplayStatus, string> = {
  pending: "office:completionGatesStatusPending",
  pass: "office:completionGatesStatusPass",
  fail: "office:completionGatesStatusFail",
  cannot_evaluate: "office:completionGatesStatusCannotEvaluate",
  needs_you: "office:completionGatesStatusNeedsYou",
  stale: "office:completionGatesStatusStale",
};

export const REASON_KEYS: Record<string, string> = {
  checks_failed: "office:completionGatesReasonChecksFailed",
  checks_missing: "office:completionGatesReasonChecksMissing",
  awaiting_checks: "office:completionGatesReasonAwaitingChecks",
  changed_files_incomplete: "office:completionGatesReasonChangedFilesIncomplete",
  review_rejected: "office:completionGatesReasonReviewRejected",
  review_approved: "office:completionGatesReasonReviewApproved",
  head_moved: "office:completionGatesReasonHeadMoved",
  pr_missing: "office:completionGatesReasonPrMissing",
  pr_merged_unverified: "office:completionGatesReasonPrMergedUnverified",
  unverified: "office:completionGatesReasonUnverified",
  evidence_stale: "office:completionGatesReasonEvidenceStale",
};

/** Button labels of the person actions. */
const ACTION_KEYS: Record<CriterionAction, string> = {
  allow: "office:completionGatesAllowForCommit",
  send_back: "office:completionGatesSendBack",
  extend_rounds: "office:completionGatesAllowMoreRounds",
  extend_attempts: "office:completionGatesAllowMoreAttempts",
  accept: "office:completionGatesAcceptAtCommit",
  approve: "office:completionGatesApproveAtCommit",
};

/** The label of an action on a criterion; a review is sent back at a commit. */
export function actionLabelKey(action: CriterionAction, kind: string | undefined): string {
  if (action === "send_back" && kind === "review") return "office:completionGatesSendBackAtCommit";
  return ACTION_KEYS[action];
}

export type CompletionDialogAction = CriterionAction | "override";

export const DIALOG_KEYS: Record<
  CompletionDialogAction,
  { title: string; description: string; confirm: string }
> = {
  override: {
    title: "office:completionGatesOverrideTitle",
    description: "office:completionGatesOverrideDescription",
    confirm: "office:completionGatesOverrideConfirm",
  },
  allow: {
    title: "office:completionGatesAllowTitle",
    description: "office:completionGatesAllowDescription",
    confirm: "office:completionGatesAllowConfirm",
  },
  send_back: {
    title: "office:completionGatesSendBackTitle",
    description: "office:completionGatesSendBackDescription",
    confirm: "office:completionGatesSendBackConfirm",
  },
  extend_rounds: {
    title: "office:completionGatesAllowMoreRounds",
    description: "office:completionGatesExtendDescription",
    confirm: "office:completionGatesExtendConfirm",
  },
  extend_attempts: {
    title: "office:completionGatesAllowMoreAttempts",
    description: "office:completionGatesExtendDescription",
    confirm: "office:completionGatesExtendConfirm",
  },
  accept: {
    title: "office:completionGatesAcceptTitle",
    description: "office:completionGatesAcceptDescription",
    confirm: "office:completionGatesAcceptAtCommit",
  },
  approve: {
    title: "office:completionGatesApproveTitle",
    description: "office:completionGatesApproveDescription",
    confirm: "office:completionGatesApproveAtCommit",
  },
};

/** Actions that record the person's reason as required. */
export const REASON_REQUIRED_ACTIONS = new Set<CompletionDialogAction>([
  "override",
  "send_back",
  "accept",
]);

const STALE_ACTION_CODE = "task_completion_evidence_changed"; // i18n-exempt: backend error code

/** The message for a person's action the backend refused. */
export function actionErrorKey(error: unknown): string {
  const body = error instanceof ApiError ? (error.body as { code?: unknown } | null) : null;
  return body?.code === STALE_ACTION_CODE
    ? "office:completionGatesActionStale"
    : "office:completionGatesActionFailed";
}
