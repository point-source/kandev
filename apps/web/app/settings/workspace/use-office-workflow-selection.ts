"use client";

import { useCallback, useEffect, useState } from "react";
import {
  getOfficeWorkflowState,
  OfficeWorkflowRefusedError,
  selectOfficeWorkflow,
  type OfficeWorkflowProblem,
  type OfficeWorkflowState,
} from "@/lib/api/domains/office-workflow-api";

type SelectionOutcome = "selected" | "refused" | "failed";

export type OfficeWorkflowSelection = {
  state: OfficeWorkflowState | null;
  problems: OfficeWorkflowProblem[];
  error: string | null;
  selecting: boolean;
  select: (workflowId: string) => Promise<SelectionOutcome>;
  reload: () => Promise<void>;
};

/**
 * Loads a workspace's Office workflow selection and candidates, and selects a
 * candidate. A refusal keeps the previous selection and exposes every
 * problem the backend named.
 */
export function useOfficeWorkflowSelection(workspaceId: string | null): OfficeWorkflowSelection {
  const [state, setState] = useState<OfficeWorkflowState | null>(null);
  const [problems, setProblems] = useState<OfficeWorkflowProblem[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [selecting, setSelecting] = useState(false);

  const reload = useCallback(async () => {
    if (!workspaceId) return;
    try {
      setState(await getOfficeWorkflowState(workspaceId));
    } catch (loadError) {
      setError(loadError instanceof Error ? loadError.message : String(loadError));
    }
  }, [workspaceId]);

  useEffect(() => {
    void reload();
  }, [reload]);

  const select = useCallback(
    async (workflowId: string): Promise<SelectionOutcome> => {
      if (!workspaceId) return "failed";
      setSelecting(true);
      setProblems([]);
      setError(null);
      try {
        setState(await selectOfficeWorkflow(workspaceId, workflowId));
        return "selected";
      } catch (selectError) {
        if (selectError instanceof OfficeWorkflowRefusedError) {
          setProblems(selectError.problems);
          return "refused";
        }
        setError(selectError instanceof Error ? selectError.message : String(selectError));
        return "failed";
      } finally {
        setSelecting(false);
      }
    },
    [workspaceId],
  );

  return { state, problems, error, selecting, select, reload };
}
