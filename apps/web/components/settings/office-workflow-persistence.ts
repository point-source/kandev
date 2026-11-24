import {
  createOfficeWorkflow,
  OfficeWorkflowRefusedError,
  saveOfficeWorkflowDefinition,
  type OfficeWorkflowDefinitionBody,
} from "@/lib/api/domains/office-workflow-api";
import { t } from "@/lib/i18n";
import { describeOfficeWorkflowProblems } from "@/lib/office-workflow/problem-messages";
import type { Workflow, WorkflowStep } from "@/lib/types/http";

const TEMP_WORKFLOW_PREFIX = "temp-workflow-";

type OfficeDraftProgress = {
  workflow?: Workflow;
  stepIds: Map<string, string>;
};

type PersistOfficeDraftParams = {
  workflow: Workflow;
  draftSteps: WorkflowStep[];
  progress: OfficeDraftProgress;
};

/** Steps without client timestamps: the backend assigns them. */
function definitionSteps(steps: WorkflowStep[]): WorkflowStep[] {
  return steps.map((step, position) => {
    const { created_at: _created, updated_at: _updated, ...rest } = step;
    return { ...rest, position } as WorkflowStep;
  });
}

function definitionBody(workflow: Workflow, steps: WorkflowStep[]): OfficeWorkflowDefinitionBody {
  return {
    name: workflow.name.trim(),
    description: workflow.description ?? "",
    prompt: workflow.prompt ?? "",
    agent_profile_id: workflow.agent_profile_id ?? "",
    steps: definitionSteps(steps),
  };
}

/**
 * Saves an Office workflow draft with one whole-definition request, so the
 * selected Office workflow never passes through a partial state. A refusal
 * becomes an error whose message lists every localized problem.
 */
export async function persistOfficeWorkflowDraft({
  workflow,
  draftSteps,
  progress,
}: PersistOfficeDraftParams): Promise<{ workflow: Workflow; steps: WorkflowStep[] }> {
  const persistedId =
    progress.workflow?.id ??
    (workflow.id.startsWith(TEMP_WORKFLOW_PREFIX) ? undefined : workflow.id);
  const body = definitionBody(workflow, draftSteps);
  let result;
  try {
    result = persistedId
      ? await saveOfficeWorkflowDefinition(persistedId, body)
      : await createOfficeWorkflow(workflow.workspace_id, body);
  } catch (error) {
    if (error instanceof OfficeWorkflowRefusedError) {
      throw new Error(describeOfficeWorkflowProblems(t, error.problems));
    }
    throw error;
  }
  draftSteps.forEach((step, index) => {
    const saved = result.steps[index];
    if (saved && saved.id !== step.id) progress.stepIds.set(step.id, saved.id);
  });
  const savedWorkflow: Workflow = {
    ...workflow,
    id: result.workflow_id as Workflow["id"],
    is_office: true,
    is_system: false,
    style: "office",
    workflow_template_id: null,
    updated_at: result.updated_at,
  };
  progress.workflow = savedWorkflow;
  return { workflow: savedWorkflow, steps: result.steps };
}
