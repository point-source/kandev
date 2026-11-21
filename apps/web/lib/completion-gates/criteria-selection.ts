import type {
  CompletionGateSettings,
  TaskCompletionCriterionRequest,
} from "@/lib/api/domains/task-completion-gates-api";

/** The editable criteria choice of the new-task dialog and settings card. */
export type CriteriaSelection = {
  ci: boolean;
  review: boolean;
  protectedPaths: string;
  reworkLimit: string;
  verificationLimit: string;
  /** Minutes a parked reviewer may wait; blank waits indefinitely. Review only. */
  reviewWaitLimit: string;
};

export const EMPTY_SELECTION: CriteriaSelection = {
  ci: false,
  review: false,
  protectedPaths: "",
  reworkLimit: "",
  verificationLimit: "",
  reviewWaitLimit: "",
};

/** Builds the selection a workspace default implies. */
export function selectionFromCriteria(
  criteria: TaskCompletionCriterionRequest[],
): CriteriaSelection {
  const ci = criteria.find((criterion) => criterion.kind === "check");
  const review = criteria.find((criterion) => criterion.kind === "review");
  const limitsSource = ci ?? review;
  return {
    ci: Boolean(ci),
    review: Boolean(review),
    protectedPaths: (ci?.protected_paths ?? []).join(", "),
    reworkLimit: limitsSource?.rework_limit ? String(limitsSource.rework_limit) : "",
    verificationLimit: limitsSource?.verification_limit
      ? String(limitsSource.verification_limit)
      : "",
    reviewWaitLimit: review?.review_wait_limit ? String(review.review_wait_limit) : "",
  };
}

function positive(value: string): number | undefined {
  const number = Number(value);
  return Number.isInteger(number) && number > 0 ? number : undefined;
}

/** Converts a selection into the request list sent to the backend. */
export function criteriaFromSelection(
  selection: CriteriaSelection,
): TaskCompletionCriterionRequest[] {
  const limits = {
    rework_limit: positive(selection.reworkLimit),
    verification_limit: positive(selection.verificationLimit),
  };
  const criteria: TaskCompletionCriterionRequest[] = [];
  if (selection.ci) {
    const paths = selection.protectedPaths
      .split(",")
      .map((path) => path.trim())
      .filter(Boolean);
    criteria.push({
      kind: "check",
      runner: "github",
      ...limits,
      ...(paths.length ? { protected_paths: paths } : {}),
    });
  }
  if (selection.review) {
    criteria.push({
      kind: "review",
      ...limits,
      review_wait_limit: positive(selection.reviewWaitLimit),
    });
  }
  return criteria.map((criterion) =>
    Object.fromEntries(Object.entries(criterion).filter(([, value]) => value !== undefined)),
  ) as TaskCompletionCriterionRequest[];
}

/** True when a default limit is outside 1..ceiling. */
export function invalidLimits(
  settings: Pick<
    CompletionGateSettings,
    | "default_verification_limit"
    | "default_rework_limit"
    | "verification_limit_ceiling"
    | "rework_limit_ceiling"
  >,
): boolean {
  const values = [
    settings.default_verification_limit,
    settings.default_rework_limit,
    settings.verification_limit_ceiling,
    settings.rework_limit_ceiling,
  ];
  if (values.some((value) => !Number.isInteger(value) || value < 1 || value > 1000)) return true;
  return (
    settings.default_verification_limit > settings.verification_limit_ceiling ||
    settings.default_rework_limit > settings.rework_limit_ceiling
  );
}

const ERROR_KEYS: Record<string, string> = {
  completion_criteria_requires_repository: "office:completionGatesErrRequiresRepository",
  completion_criteria_requires_review_stage: "office:completionGatesErrRequiresReviewStage",
  completion_limit_above_ceiling: "office:completionGatesErrLimitAboveCeiling",
  invalid_protected_path: "office:completionGatesErrInvalidProtectedPath",
};

/** The translation key for a creation refusal code, if it is one. */
export function completionCriteriaErrorKey(message: string): string | undefined {
  return ERROR_KEYS[message.trim()];
}
