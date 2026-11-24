import type { OfficeWorkflowProblem } from "@/lib/api/domains/office-workflow-api";

/** A translate function such as `t` from `@/lib/i18n` or `useTranslation()`. */
export type Translate = (key: string, options?: Record<string, unknown>) => string;

type ProblemValues = { step: string; role: string; action: string; count: number; detail: string };

function problemValues(problem: OfficeWorkflowProblem): ProblemValues {
  return {
    step: problem.step_name ?? "",
    role: problem.role ?? "",
    action: problem.action ?? "",
    count: problem.count ?? 0,
    detail: problem.detail ?? "",
  };
}

/**
 * Renders one Office workflow problem as a localized sentence that states
 * the problem and its fix. Unknown codes fall back to a generic sentence.
 */
export function describeOfficeWorkflowProblem(
  t: Translate,
  problem: OfficeWorkflowProblem,
): string {
  const v = problemValues(problem);
  switch (problem.code) {
    case "start_step_count":
      return t("workflows:officeProblemStartStepCount", { count: v.count });
    case "missing_work_stage":
      return t("workflows:officeProblemMissingWorkStage");
    case "final_step_not_completing":
      return t("workflows:officeProblemFinalStepNotCompleting", v);
    case "completing_step_unreachable":
      return t("workflows:officeProblemCompletingStepUnreachable", v);
    case "missing_seat_action":
      return t("workflows:officeProblemMissingSeatAction", v);
    case "unknown_agent_profile":
      return t("workflows:officeProblemUnknownAgentProfile", v);
    default:
      return describeRefusal(t, problem, v);
  }
}

function describeRefusal(t: Translate, problem: OfficeWorkflowProblem, v: ProblemValues): string {
  switch (problem.code) {
    case "not_in_workspace":
      return t("workflows:officeProblemNotInWorkspace");
    case "not_selectable":
      return t("workflows:officeProblemNotSelectable");
    case "sync_managed":
      return t("workflows:officeProblemSyncManaged");
    case "holds_kanban_tasks":
      return t("workflows:officeProblemHoldsKanbanTasks");
    case "requires_office_workspace":
      return t("workflows:officeProblemRequiresOfficeWorkspace");
    case "built_in_read_only":
      return t("workflows:officeProblemBuiltInReadOnly");
    case "selected_delete":
      return t("workflows:officeProblemSelectedDelete");
    case "step_holds_tasks":
      return t("workflows:officeProblemStepHoldsTasks", v);
    case "selected_requires_definition_save":
      return t("workflows:officeProblemSelectedRequiresDefinitionSave");
    case "invalid_events":
      return t("workflows:officeProblemInvalidEvents", v);
    case "not_office_workflow":
      return t("workflows:officeProblemNotOfficeWorkflow");
    default:
      return t("workflows:officeProblemUnknown", { code: problem.code });
  }
}

/** Joins every problem sentence for a toast or error description. */
export function describeOfficeWorkflowProblems(
  t: Translate,
  problems: OfficeWorkflowProblem[],
): string {
  return problems.map((problem) => describeOfficeWorkflowProblem(t, problem)).join("\n");
}
