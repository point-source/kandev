/** Workflow notification payload (`workflow.created` / `workflow.updated`). */
export type WorkflowPayload = {
  id: string;
  workspace_id: string;
  name: string;
  description?: string;
  prompt?: string;
  agent_profile_id?: string;
  source?: string;
  source_path?: string;
  hidden?: boolean;
  style?: "kanban" | "office" | "custom"; // Phase 2 (ADR-0004) UX hint, frontend-only.
  is_office?: boolean; // Office workflows never appear in Kanban listings.
  created_at?: string;
  updated_at?: string;
};
