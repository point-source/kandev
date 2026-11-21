import type {
  CompletionGateSettings,
  TaskCompletionCriterionRequest,
} from "@/lib/api/domains/task-completion-gates-api";

/** The editable criteria choice of the new-task dialog and settings card. */
export type CriteriaSelection = {
  ci: boolean;
  review: boolean;
  protectedPaths: string;
  maxRework: string;
  maxAttempts: string;
};

export const EMPTY_SELECTION: CriteriaSelection = {
  ci: false,
  review: false,
  protectedPaths: "",
  maxRework: "",
  maxAttempts: "",
};

/** Builds the selection a workspace default implies. */
export function selectionFromCriteria(
  criteria: TaskCompletionCriterionRequest[],
): CriteriaSelection {
  const ci = criteria.find((criterion) => criterion.kind === "ci_at_head");
  const review = criteria.find((criterion) => criterion.kind === "review_at_head");
  const capsSource = ci ?? review;
  return {
    ci: Boolean(ci),
    review: Boolean(review),
    protectedPaths: (ci?.protected_paths ?? []).join(", "),
    maxRework: capsSource?.max_rework_rounds ? String(capsSource.max_rework_rounds) : "",
    maxAttempts: capsSource?.max_verification_attempts
      ? String(capsSource.max_verification_attempts)
      : "",
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
  const caps = {
    max_rework_rounds: positive(selection.maxRework),
    max_verification_attempts: positive(selection.maxAttempts),
  };
  const criteria: TaskCompletionCriterionRequest[] = [];
  if (selection.ci) {
    const paths = selection.protectedPaths
      .split(",")
      .map((path) => path.trim())
      .filter(Boolean);
    criteria.push({
      kind: "ci_at_head",
      ...caps,
      ...(paths.length ? { protected_paths: paths } : {}),
    });
  }
  if (selection.review) criteria.push({ kind: "review_at_head", ...caps });
  return criteria.map((criterion) =>
    Object.fromEntries(Object.entries(criterion).filter(([, value]) => value !== undefined)),
  ) as TaskCompletionCriterionRequest[];
}

/** True when a default limit is outside 1..ceiling. */
export function invalidLimits(
  settings: Pick<
    CompletionGateSettings,
    "default_max_attempts" | "default_max_rework" | "ceiling_attempts" | "ceiling_rework"
  >,
): boolean {
  const values = [
    settings.default_max_attempts,
    settings.default_max_rework,
    settings.ceiling_attempts,
    settings.ceiling_rework,
  ];
  if (values.some((value) => !Number.isInteger(value) || value < 1 || value > 1000)) return true;
  return (
    settings.default_max_attempts > settings.ceiling_attempts ||
    settings.default_max_rework > settings.ceiling_rework
  );
}

const ERROR_KEYS: Record<string, string> = {
  completion_criteria_requires_repository: "office:completionGatesErrRequiresRepository",
  completion_criteria_requires_review_stage: "office:completionGatesErrRequiresReviewStage",
  completion_cap_above_ceiling: "office:completionGatesErrCapAboveCeiling",
  invalid_protected_path: "office:completionGatesErrInvalidProtectedPath",
};

/** The translation key for a creation refusal code, if it is one. */
export function completionCriteriaErrorKey(message: string): string | undefined {
  return ERROR_KEYS[message.trim()];
}
