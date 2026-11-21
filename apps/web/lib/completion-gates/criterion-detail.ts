import type {
  EffectiveCompletionLimits,
  TaskCompletionCriterion,
} from "@/lib/api/domains/task-completion-gates-api";
import { REASON_KEYS } from "./copy-keys";
import { shortHeads } from "./criteria-display";

/** The fields of a task decision the detail line reads. */
export type DecisionLike = {
  id: string;
  deciderType: "user" | "agent";
  deciderId: string;
  deciderName: string;
  decision: string;
};

export type Reviewer = { id: string; name: string; type: "user" | "agent" };

/**
 * What one criterion row says under its name: a translated text or the
 * reviewers who decided it, and the commits it was decided at.
 */
export type CriterionDetail = {
  commits: string[];
  text?: { key: string; values?: Record<string, string | number> };
  reviewers?: { key: string; reviewers: Reviewer[] };
};

type Described = Omit<CriterionDetail, "commits">;

const NO_REVIEWER_KEYS: Record<string, string> = {
  assignee_only: "office:completionGatesHoldNoReviewerAssigneeOnly",
  no_reviewer_role: "office:completionGatesHoldNoReviewerNoRole",
  author_provider_only: "office:completionGatesHoldNoReviewerAuthorProviderOnly",
};

export function criterionDetail(
  criterion: TaskCompletionCriterion,
  blocker: string | undefined,
  decisions: DecisionLike[],
  limits?: EffectiveCompletionLimits,
): CriterionDetail {
  const commits = shortHeads(
    criterion.evidence?.subject.revision ?? criterion.result_subject_revision,
  );
  return { commits, ...describe(criterion, blocker, decisions, limits) };
}

function describe(
  criterion: TaskCompletionCriterion,
  blocker: string | undefined,
  decisions: DecisionLike[],
  limits?: EffectiveCompletionLimits,
): Described {
  if (criterion.hold_kind) return describeHold(criterion, limits);
  const first = criterion.result_detail?.[0] ?? "";
  switch (criterion.result_reason) {
    case "sent_back":
      return text("office:completionGatesDetailSentBack", { reason: first });
    case "accepted":
      return text("office:completionGatesDetailAccepted", { reason: first });
    case "waiting_for_reviewer":
      return describeReviewerWait(criterion.result_detail?.[1]);
  }
  if (criterion.kind === "review") return describeReview(criterion, blocker, decisions);
  if (!blocker) return {};
  if (criterion.result_reason === "checks_failed") {
    return text("office:completionGatesDetailFailing", { checks: joinedDetail(criterion) });
  }
  if (criterion.result_reason === "awaiting_checks" || !criterion.result) return {};
  return reasonText(criterion.result_reason ?? blocker);
}

function describeHold(
  criterion: TaskCompletionCriterion,
  limits?: EffectiveCompletionLimits,
): Described {
  const detail = criterion.result_detail ?? [];
  switch (criterion.hold_kind) {
    case "protected_files_changed":
      return text("office:completionGatesHoldProtectedFilesChanged", {
        files: joinedDetail(criterion),
        count: detail.length,
      });
    case "rework_limit_reached": {
      const used = criterion.rework_count ?? 0;
      return text("office:completionGatesHoldReworkLimitReached", {
        used,
        limit: limits?.rework_limit ?? used,
      });
    }
    case "verification_limit_reached": {
      const used = criterion.attempt_count ?? 0;
      return text("office:completionGatesHoldVerificationLimitReached", {
        used,
        limit: limits?.verification_limit ?? used,
      });
    }
    case "no_reviewer_available":
      return text(NO_REVIEWER_KEYS[detail[0] ?? ""] ?? NO_REVIEWER_KEYS.no_reviewer_role);
    case "review_wait_limit_reached":
      return describeWaited(detail[0], criterion.held_at);
  }
  return {};
}

/** How long a review waited for a parked reviewer, from its start to the hold. */
function describeWaited(since: string | undefined, heldAt: string | null | undefined): Described {
  const start = Date.parse(since ?? "");
  const end = Date.parse(heldAt ?? "");
  const minutes =
    Number.isNaN(start) || Number.isNaN(end) ? 0 : Math.max(0, Math.round((end - start) / 60000));
  if (minutes >= 60)
    return text("office:completionGatesHoldReviewWaitHours", { value: Math.round(minutes / 60) });
  return text("office:completionGatesHoldReviewWaitMinutes", { value: minutes });
}

function describeReviewerWait(retryAt: string | undefined): Described {
  const retry = Date.parse(retryAt ?? "");
  if (Number.isNaN(retry)) return text("office:completionGatesDetailWaitingForReviewer");
  const time = new Date(retry).toLocaleTimeString(undefined, {
    hour: "2-digit",
    minute: "2-digit",
  });
  return text("office:completionGatesDetailWaitingForReviewerUntil", { time });
}

function describeReview(
  criterion: TaskCompletionCriterion,
  blocker: string | undefined,
  decisions: DecisionLike[],
): Described {
  if (!blocker) {
    const ids = new Set((criterion.evidence?.reference ?? "").split(",").filter(Boolean));
    return reviewers(
      "office:completionGatesDetailApprovedBy",
      decisions.filter((decision) => ids.has(decision.id)),
      "review_approved",
    );
  }
  if (criterion.result === "fail") {
    return reviewers(
      "office:completionGatesDetailChangesRequestedBy",
      decisions.filter((decision) => decision.decision === "changes_requested"),
      "review_rejected",
    );
  }
  return text("office:completionGatesDetailNeedsApproval");
}

function reviewers(key: string, deciding: DecisionLike[], fallbackReason: string): Described {
  if (deciding.length === 0) return reasonText(fallbackReason);
  return {
    reviewers: {
      key,
      reviewers: deciding.map((decision) => ({
        id: decision.deciderId,
        name: decision.deciderName,
        type: decision.deciderType,
      })),
    },
  };
}

function joinedDetail(criterion: TaskCompletionCriterion): string {
  return (criterion.result_detail ?? []).join(", ");
}

function text(key: string, values?: Record<string, string | number>): Described {
  return { text: values ? { key, values } : { key } };
}

function reasonText(reason: string): Described {
  const key = REASON_KEYS[reason];
  return key ? text(key) : {};
}
