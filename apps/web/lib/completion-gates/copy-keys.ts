// Translation keys for completion-gate values. Keys, not copy: components
// resolve them with t() at render.
import type { CriterionDisplayStatus } from "./criteria-display";

export const KIND_KEYS: Record<string, string> = {
  ci_at_head: "office:completionGatesKindCi",
  review_at_head: "office:completionGatesKindReview",
  attested: "office:completionGatesKindAttested",
};

export const STATUS_KEYS: Record<CriterionDisplayStatus, string> = {
  pending: "office:completionGatesStatusPending",
  pass: "office:completionGatesStatusPass",
  fail: "office:completionGatesStatusFail",
  cannot_evaluate: "office:completionGatesStatusCannotEvaluate",
  held: "office:completionGatesStatusHeld",
  stale: "office:completionGatesStatusStale",
};

export const REASON_KEYS: Record<string, string> = {
  checks_failed: "office:completionGatesReasonChecksFailed",
  checks_missing: "office:completionGatesReasonChecksMissing",
  awaiting_checks: "office:completionGatesReasonAwaitingChecks",
  changed_files_incomplete: "office:completionGatesReasonChangedFilesIncomplete",
  protected_path_changed: "office:completionGatesReasonProtectedPathChanged",
  rework_cap_reached: "office:completionGatesReasonReworkCapReached",
  attempt_cap_reached: "office:completionGatesReasonAttemptCapReached",
  review_rejected: "office:completionGatesReasonReviewRejected",
  review_approved: "office:completionGatesReasonReviewApproved",
  head_moved: "office:completionGatesReasonHeadMoved",
  pr_missing: "office:completionGatesReasonPrMissing",
  pr_merged_unverified: "office:completionGatesReasonPrMergedUnverified",
  unverified: "office:completionGatesReasonUnverified",
  evidence_stale: "office:completionGatesReasonEvidenceStale",
};

export type CompletionDialogAction = "override" | "allow" | "send_back" | "extend";

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
  extend: {
    title: "office:completionGatesExtendTitle",
    description: "office:completionGatesExtendDescription",
    confirm: "office:completionGatesExtendConfirm",
  },
};
