import type { ForegroundActivity } from "@/lib/types/http";

/**
 * Payload for the `session.state_changed` WS event. Extracted from the
 * backend message-map registry to keep that file under its line cap; re-exported
 * from `@/lib/types/backend` for existing importers.
 */
export type TaskSessionStateChangedPayload = {
  task_id: string;
  session_id: string;
  old_state?: string;
  new_state?: string;
  /** Authoritative row timestamp — used to drop out-of-order subscribe snapshots. */
  updated_at?: string;
  /**
   * Agent profile id — drives the per-agent live-session selectors on the
   * sidebar. Empty for sessions launched without a profile.
   */
  agent_profile_id?: string;
  agent_profile_snapshot?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
  session_metadata?: Record<string, unknown>;
  is_passthrough?: boolean;
  error_message?: string;
  /** When true, the frontend should not show an error toast for this state change. */
  suppress_toast?: boolean;
  // Workflow-related fields (sent during workflow transitions)
  review_status?: string;
  // Task environment (for session→environment mapping)
  task_environment_id?: string;
  // Fine-grained busy substate (§spec:fine-grained-busy-signal), carried on
  // every transition so the client resets stale values; intra-RUNNING flips
  // arrive on session.activity_changed instead.
  foreground_activity?: ForegroundActivity;
};

/**
 * Payload for `session.activity_changed` — the fine-grained busy signal
 * (§spec:fine-grained-busy-signal). Fires when a RUNNING session's foreground
 * turn flips between generating and idle-on-background-work, with no coarse
 * state change.
 */
export type TaskSessionActivityChangedPayload = {
  task_id: string;
  session_id: string;
  foreground_activity: ForegroundActivity;
};
