import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import { useTask } from "@/hooks/use-task";
import type { TaskSession } from "@/lib/types/http";

export function deriveSessionFlags(session: TaskSession | null | undefined) {
  const state = session?.state;
  const errorMessage = session?.error_message;
  const foregroundActivity = session?.foreground_activity;
  const isStarting = state === "STARTING";
  const isRunning = state === "RUNNING";
  // §spec:fine-grained-busy-signal. Three conditions, not two:
  //  (a) generating       — RUNNING, foreground actively producing output
  //  (b) background-idle   — RUNNING, foreground yielded to spawned background work
  //  (c) fully idle        — not RUNNING
  // Only (a) gates the composer; (b) accepts input while the "working"
  // affordance stays up. An absent/unknown substate defaults to generating,
  // preserving the historical reject-while-RUNNING contract.
  const isBackgroundWorking = isRunning && foregroundActivity === "background";
  const isForegroundGenerating = isRunning && !isBackgroundWorking;
  // "Busy" for input-gating: only a foreground-generating turn blocks the
  // composer. In (b) the operator can type; the message sends, not queues.
  const isAgentBusy = isForegroundGenerating;
  // "Working" drives the spinner/affordance: any live turn (generating OR
  // background-idle) plus STARTING — it must stay up through (b).
  const isWorking = isStarting || isRunning;
  const isFailed = state === "FAILED" || state === "CANCELLED";
  const needsRecovery = state === "WAITING_FOR_INPUT" && !!errorMessage;
  return {
    isStarting,
    isWorking,
    isAgentBusy,
    isForegroundGenerating,
    isBackgroundWorking,
    isFailed,
    needsRecovery,
  };
}

type UseSessionStateOptions = {
  taskIdHint?: string | null;
};

export function useSessionState(sessionId: string | null, options: UseSessionStateOptions = {}) {
  const { taskIdHint = null } = options;
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);

  // Validate that active session belongs to the active task before using it.
  // This prevents showing messages from an unrelated session when navigating
  // to a task that has no sessions yet (activeSessionId may still hold the
  // old session from the previous task).
  const activeSessionData = useAppStore((state) =>
    activeSessionId ? (state.taskSessions.items[activeSessionId] ?? null) : null,
  );
  const validatedActiveSessionId =
    activeSessionData && activeSessionData.task_id === activeTaskId ? activeSessionId : null;

  const resolvedSessionId = sessionId ?? validatedActiveSessionId;

  const { session } = useSession(resolvedSessionId);
  const taskId = session?.task_id ?? taskIdHint ?? null;
  const task = useTask(taskId);
  const prepareStatus = useAppStore((state) =>
    resolvedSessionId ? state.prepareProgress.bySessionId[resolvedSessionId]?.status : undefined,
  );

  const taskDescription = task?.description ?? null;
  const flags = deriveSessionFlags(session);
  const isPreparingEnvironment = prepareStatus === "preparing";

  return {
    resolvedSessionId,
    session,
    task,
    taskId,
    taskDescription,
    ...flags,
    isStarting: flags.isStarting || isPreparingEnvironment,
    isWorking: flags.isWorking || isPreparingEnvironment,
    // Exposed separately so consumers that gate on "is the executor still
    // bootstrapping" (e.g. the chat input "agent is still being set up"
    // tooltip) can distinguish a brief STARTING transition — which every
    // session passes through, including local quick-chat — from an active
    // Docker / Sprites prepare-environment phase.
    isPreparingEnvironment,
  };
}
