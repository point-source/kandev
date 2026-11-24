"use client";

import { useMemo, useState } from "react";
import { deleteWorkflowAction } from "@/app/actions/workspaces";
import { workflowIsDirty } from "@/hooks/domains/settings/use-workflow-settings";
import {
  toOfficeWorkflowError,
  OfficeWorkflowRefusedError,
} from "@/lib/api/domains/office-workflow-api";
import { t } from "@/lib/i18n";
import { describeOfficeWorkflowProblems } from "@/lib/office-workflow/problem-messages";
import {
  agentProfileId,
  workflowId as toWorkflowId,
  type Workflow,
  type WorkflowStep,
} from "@/lib/types/http";
import type { WorkflowSavedParams } from "./workspace-workflows-list";
import { createWorkflowDuplication } from "./workflow-duplication";

const TEMP_WORKFLOW_PREFIX = "temp-workflow-";

type WorkflowMetadataUpdate = {
  name?: string;
  description?: string;
  prompt?: string;
  agent_profile_id?: string;
};

function insertAfter(items: Workflow[], sourceId: string, draft: Workflow): Workflow[] {
  const index = items.findIndex((item) => item.id === sourceId);
  if (index === -1) return [...items, draft];
  return [...items.slice(0, index + 1), draft, ...items.slice(index + 1)];
}

async function deletePersistedWorkflow(workflowId: string) {
  try {
    await deleteWorkflowAction(workflowId);
  } catch (error) {
    const converted = toOfficeWorkflowError(error);
    if (converted instanceof OfficeWorkflowRefusedError) {
      throw new Error(describeOfficeWorkflowProblems(t, converted.problems));
    }
    throw converted;
  }
}

/**
 * Draft state for a workspace's Office workflows: the built-in and its
 * copies. Duplicates are route-local drafts until saved.
 */
export function useOfficeWorkflowItems(initialWorkflows: Workflow[]) {
  const [items, setItems] = useState<Workflow[]>(initialWorkflows);
  const [savedItems, setSavedItems] = useState<Workflow[]>(initialWorkflows);
  const [initialSteps, setInitialSteps] = useState(() => new Map<string, WorkflowStep[]>());
  const [builtInCopyIds, setBuiltInCopyIds] = useState(() => new Set<string>());
  const savedById = useMemo(() => new Map(savedItems.map((item) => [item.id, item])), [savedItems]);

  const duplicate = (source: Workflow, sourceSteps: WorkflowStep[]) => {
    const { workflow, steps } = createWorkflowDuplication(source, items, sourceSteps);
    setItems((previous) => insertAfter(previous, source.id, workflow));
    setInitialSteps((previous) => new Map(previous).set(workflow.id, steps));
    if (source.is_system) setBuiltInCopyIds((previous) => new Set(previous).add(workflow.id));
  };

  const update = (workflowId: string, updates: WorkflowMetadataUpdate) => {
    setItems((previous) =>
      previous.map((item) =>
        item.id === workflowId
          ? {
              ...item,
              ...updates,
              agent_profile_id:
                updates.agent_profile_id !== undefined
                  ? agentProfileId(updates.agent_profile_id)
                  : item.agent_profile_id,
            }
          : item,
      ),
    );
  };

  const forget = (workflowId: string) => {
    setItems((previous) => previous.filter((item) => item.id !== workflowId));
    setSavedItems((previous) => previous.filter((item) => item.id !== workflowId));
    setBuiltInCopyIds((previous) => {
      const next = new Set(previous);
      next.delete(workflowId);
      return next;
    });
  };

  const remove = async (workflowId: string) => {
    if (!workflowId.startsWith(TEMP_WORKFLOW_PREFIX)) await deletePersistedWorkflow(workflowId);
    forget(workflowId);
  };

  const saved = ({
    clientWorkflow,
    savedWorkflow,
    currentSteps,
    finalizeIdentity,
  }: WorkflowSavedParams) => {
    const isNew = clientWorkflow.id.startsWith(TEMP_WORKFLOW_PREFIX);
    if (isNew && !finalizeIdentity) return;
    setItems((previous) =>
      previous.map((item) => (item.id === clientWorkflow.id ? savedWorkflow : item)),
    );
    setSavedItems((previous) => [
      ...previous.filter((item) => item.id !== savedWorkflow.id),
      savedWorkflow,
    ]);
    if (isNew) {
      setInitialSteps((previous) => new Map(previous).set(savedWorkflow.id, currentSteps));
      setBuiltInCopyIds((previous) => {
        const next = new Set(previous);
        next.delete(clientWorkflow.id);
        return next;
      });
    }
  };

  const discard = (workflowId: string) => {
    if (workflowId.startsWith(TEMP_WORKFLOW_PREFIX)) {
      forget(workflowId);
      return;
    }
    const baseline = savedById.get(toWorkflowId(workflowId));
    if (baseline)
      setItems((previous) => previous.map((item) => (item.id === workflowId ? baseline : item)));
  };

  return {
    items,
    savedItems,
    initialSteps,
    builtInCopyIds,
    isWorkflowDirty: (workflow: Workflow) => workflowIsDirty(workflow, savedById),
    duplicate,
    update,
    remove,
    saved,
    discard,
  };
}
