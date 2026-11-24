import {
  acceptCriterion,
  allowProtectedFiles,
  approveCriterion,
  extendCompletionCriterion,
  sendBackCriterion,
} from "@/lib/api/domains/task-completion-gates-api";
import type { TaskCompletionCriterion } from "@/lib/api/domains/task-completion-gates-api";
import type { CriterionAction } from "./criteria-display";

/** The criterion a person acts on, at the revision and heads they saw. */
export type ActionTarget = {
  taskId: string;
  criterionId: string;
  criterionRevision: number;
  subjectRevision: string;
};

/** The target of an action on a criterion as shown to the person. */
export function actionTarget(taskId: string, criterion: TaskCompletionCriterion): ActionTarget {
  return {
    taskId,
    criterionId: criterion.id,
    criterionRevision: criterion.criterion_revision,
    subjectRevision: criterion.result_subject_revision ?? "",
  };
}

export type ActionValues = { reason: string; rework: number; attempts: number };

/** Applies a person's action; the backend refuses it when the heads moved. */
export async function applyCriterionAction(
  target: ActionTarget,
  action: CriterionAction,
  values: ActionValues,
): Promise<void> {
  const { taskId, criterionId } = target;
  if (action === "extend_rounds" || action === "extend_attempts") {
    await extendCompletionCriterion(taskId, criterionId, {
      expected_criterion_revision: target.criterionRevision,
      rework_limit: values.rework || undefined,
      verification_limit: values.attempts || undefined,
      reason: values.reason,
    });
    return;
  }
  const request = {
    expected_criterion_revision: target.criterionRevision,
    expected_subject_revision: target.subjectRevision,
    reason: values.reason,
  };
  const post = {
    allow: allowProtectedFiles,
    send_back: sendBackCriterion,
    accept: acceptCriterion,
    approve: approveCriterion,
  }[action];
  await post(taskId, criterionId, request);
}

/** The limit an action's dialog asks for: rounds or attempts for an extension. */
export function limitField(action: string): "rework" | "attempts" | undefined {
  if (action === "extend_rounds") return "rework";
  if (action === "extend_attempts") return "attempts";
  return undefined;
}
