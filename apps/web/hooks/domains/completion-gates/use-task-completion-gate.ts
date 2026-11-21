import { useCallback, useEffect, useRef, useState } from "react";
import {
  getEffectiveCompletionLimits,
  getTaskCompletionGate,
  listTaskCompletionGateHistory,
  type EffectiveCompletionLimits,
  type TaskCompletionGateHistory,
  type TaskCompletionGateSnapshot,
} from "@/lib/api/domains/task-completion-gates-api";
import { fetchTask, moveTask } from "@/lib/api/domains/kanban-api";
import { listWorkflowSteps } from "@/lib/api/domains/workflow-api";
import { completingStepId } from "@/lib/completion-gates/criteria-display";

type GateState = {
  gate: TaskCompletionGateSnapshot | null;
  history: TaskCompletionGateHistory[];
  limits: EffectiveCompletionLimits[];
  error: boolean;
};

/**
 * Loads a gated task's completion gate, history and effective limits. It reads
 * nothing when the task has no criteria and refetches when refreshKey changes
 * (built from the gate's state revision, bumped by every gate change). Limits
 * are optional: a failed limits read leaves the gate visible.
 */
export function useTaskCompletionGate(taskId: string, enabled: boolean, refreshKey: string) {
  const [state, setState] = useState<GateState>({
    gate: null,
    history: [],
    limits: [],
    error: false,
  });
  const generation = useRef(0);

  const load = useCallback(async () => {
    const current = ++generation.current;
    try {
      const [gate, history, limits] = await Promise.all([
        getTaskCompletionGate(taskId),
        listTaskCompletionGateHistory(taskId),
        getEffectiveCompletionLimits(taskId).catch(() => [] as EffectiveCompletionLimits[]),
      ]);
      if (current === generation.current) setState({ gate, history, limits, error: false });
    } catch {
      if (current === generation.current) setState((prev) => ({ ...prev, error: true }));
    }
  }, [taskId]);

  useEffect(() => {
    if (!enabled) return;
    void load();
  }, [enabled, load, refreshKey]);

  return { ...state, reload: load };
}

/** Moves the task into its completing step with a recorded override. */
export async function overrideTaskCompletion(taskId: string, gateRevision: number, reason: string) {
  const task = await fetchTask(taskId);
  const { steps } = await listWorkflowSteps(task.workflow_id);
  const target = completingStepId(steps);
  if (!target) throw new Error("no completing step"); // i18n-exempt: internal error surfaced as a generic message
  return moveTask(taskId, {
    workflow_id: task.workflow_id,
    workflow_step_id: target,
    completion_override: { expected_revision: gateRevision, reason },
  });
}
