import type { TaskCompletionCriterion } from "@/lib/api/domains/task-completion-gates-api";
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

const PROTECTED_HOLD = "protected_path_changed";

export function criterionDetail(
  criterion: TaskCompletionCriterion,
  blocker: string | undefined,
  decisions: DecisionLike[],
): CriterionDetail {
  const commits = shortHeads(
    criterion.evidence?.subject.revision ?? criterion.result_subject_revision,
  );
  return { commits, ...describe(criterion, blocker, decisions) };
}

function describe(
  criterion: TaskCompletionCriterion,
  blocker: string | undefined,
  decisions: DecisionLike[],
): Described {
  const detail = (criterion.result_detail ?? []).join(", ");
  if (criterion.held_reason === PROTECTED_HOLD) {
    return text("office:completionGatesDetailProtectedChanged", {
      files: detail,
      count: criterion.result_detail?.length ?? 0,
    });
  }
  if (criterion.held_reason) return reasonText(criterion.held_reason);
  if (criterion.kind === "review_at_head") return describeReview(criterion, blocker, decisions);
  if (!blocker) return {};
  switch (criterion.result_reason) {
    case "checks_failed":
      return text("office:completionGatesDetailFailing", { checks: detail });
    case "sent_back":
      return text("office:completionGatesDetailSentBack", { reason: detail });
    case "awaiting_checks":
      return {};
  }
  if (!criterion.result) return {};
  return reasonText(criterion.result_reason ?? blocker);
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

function text(key: string, values?: Record<string, string | number>): Described {
  return { text: values ? { key, values } : { key } };
}

function reasonText(reason: string): Described {
  const key = REASON_KEYS[reason];
  return key ? text(key) : {};
}
