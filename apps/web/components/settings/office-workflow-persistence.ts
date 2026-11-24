import {
  createOfficeWorkflow,
  getOfficeWorkflowState,
  isWorkflowChangedError,
  OfficeWorkflowRefusedError,
  saveOfficeWorkflowDefinition,
  type OfficeWorkflowDefinitionBody,
} from "@/lib/api/domains/office-workflow-api";
import { t } from "@/lib/i18n";
import {
  describeOfficeWorkflowProblem,
  describeOfficeWorkflowProblems,
} from "@/lib/office-workflow/problem-messages";
import type { Workflow, WorkflowStep } from "@/lib/types/http";

const TEMP_WORKFLOW_PREFIX = "temp-workflow-";

type OfficeDraftProgress = {
  workflow?: Workflow;
  stepIds: Map<string, string>;
};

type PersistOfficeDraftParams = {
  workflow: Workflow;
  draftSteps: WorkflowStep[];
  /** The steps as last loaded or saved; their versions fence the save. */
  savedSteps: WorkflowStep[];
  progress: OfficeDraftProgress;
};

/** Steps without client timestamps or drafts: the backend assigns versions. */
function definitionSteps(steps: WorkflowStep[]): WorkflowStep[] {
  return steps.map((step, position) => {
    const {
      created_at: _created,
      updated_at: _updated,
      office_events_draft: _draft,
      ...rest
    } = step;
    return { ...rest, position } as WorkflowStep;
  });
}

/** The versions this editor loaded, so a save never overwrites another writer. */
function expectedVersions(workflow: Workflow, savedSteps: WorkflowStep[]) {
  return {
    expected_updated_at: workflow.updated_at,
    expected_steps: Object.fromEntries(savedSteps.map((step) => [step.id, step.updated_at])),
  };
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
  savedSteps,
  progress,
}: PersistOfficeDraftParams): Promise<{ workflow: Workflow; steps: WorkflowStep[] }> {
  const persistedId =
    progress.workflow?.id ??
    (workflow.id.startsWith(TEMP_WORKFLOW_PREFIX) ? undefined : workflow.id);
  const body = definitionBody(workflow, draftSteps);
  let result;
  try {
    result = persistedId
      ? await saveOfficeWorkflowDefinition(persistedId, {
          ...body,
          ...expectedVersions(workflow, savedSteps),
        })
      : await createOfficeWorkflow(workflow.workspace_id, body);
  } catch (error) {
    if (error instanceof OfficeWorkflowRefusedError) {
      throw new Error(describeOfficeWorkflowProblems(t, error.problems));
    }
    if (isWorkflowChangedError(error)) {
      throw new Error(t("workflows:officeWorkflowChangedElsewhere"));
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

/**
 * Refuses deleting the workspace's selected Office workflow before any task
 * moves, including a selection made after the delete dialog opened.
 */
export async function ensureOfficeWorkflowDeletable(workflow: Workflow): Promise<void> {
  if (!workflow.is_office) return;
  const state = await getOfficeWorkflowState(workflow.workspace_id);
  if (state.selected_workflow_id === workflow.id) {
    throw new Error(describeOfficeWorkflowProblem(t, { code: "selected_delete" }));
  }
}
