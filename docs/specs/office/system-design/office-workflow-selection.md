---
status: current
system: office
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-001
  - REQ-OFFICE-WORKFLOW-SELECT-002
  - REQ-OFFICE-WORKFLOW-SELECT-003
  - REQ-OFFICE-WORKFLOW-SELECT-004
  - REQ-OFFICE-WORKFLOW-SELECT-005
  - REQ-OFFICE-WORKFLOW-SELECT-006
  - REQ-OFFICE-WORKFLOW-SELECT-007
---

# Office Workflow Selection System Design

## Context and boundaries

Today three things decide "Office" behavior, and all of them compare against
`workspaces.office_workflow_id`:

- **Task identity.** `isFromOfficeProjection` in
  `internal/task/repository/sqlite/task.go` (exported as
  `IsFromOfficePredicate`) marks a task Office when it has a `project_id`,
  when `tasks.workflow_id = workspaces.office_workflow_id`, or when it sits on
  the workspace's system `routine` workflow. `models.Task.IsFromOffice` is that
  projection.
- **Kanban exclusion.** `Service.GetOfficeWorkflowIDs`
  (`internal/task/service/service_resources.go`) collects every workspace's
  `office_workflow_id`; `WorkflowHandlers.listWorkflows` (`excludeOffice`) and
  `backendapp/boot_state.go` drop those workflows from Kanban listings.
- **Equality guards.** `Service.validateAssigneeTaskIsOffice`
  (`internal/task/service/service_tasks.go`) and the cross-workspace handoff
  check in `internal/office/runtime/handoff.go` compare a workflow ID against
  `office_workflow_id`.

New Office tasks get their workflow from `Service.resolveOfficeWorkflow`
(`GetWorkspaceTaskPrefix` returns `office_workflow_id`). Onboarding writes the
column once through `Repository.EnsureOfficeWorkflow` /
`stampWorkspaceOfficeWorkflow` (`internal/task/repository/sqlite/defaults.go`).
No API writes it.

If only the column changed, tasks on the previous workflow would stop being
Office tasks and that workflow would come back onto the board. This design
separates **membership** (which workflows are Office workflows) from
**selection** (which one new Office tasks use), per
[ADR 2026-11-24 Office workflow marker](../../../decisions/2026-11-24-office-workflow-marker.md).

Owned by the task system and reused unchanged: workflow and step storage
(`internal/workflow/repository`), step event validation
(`models.ValidateStepEventsWithRouting`), the portable format
(`internal/workflow/models/export.go`), import profile selection, and
client-side workflow duplication (`app/settings/workspace/workflow-duplication.ts`).

**Step-name audit.** The only Office code that matches `office-default` steps
by name or template ID is the built-in healing in `defaults.go`
(`healBuiltinWorkflowStepFlags`) and
`builtin_workflow_seat_reconcile.go`; both are limited to `is_system = 1`
rows, so they never touch Office copies. Office routing, seat fan-out, review
stage resolution (`office/service/review_stage.go`), prompts and stall
detection read `stage_type` and workflow actions. The Office system-task
projection (`office/repository/sqlite/tasks.go`) keys on the `routine`
template only. No further step-name dependency needs changing.

## Requirement mapping

| Requirement | Sections |
| --- | --- |
| REQ-OFFICE-WORKFLOW-SELECT-001 | Office marker; Membership readers; Migration |
| REQ-OFFICE-WORKFLOW-SELECT-002 | Settings Workflows tab; Definition save; Deletion guard |
| REQ-OFFICE-WORKFLOW-SELECT-003 | Office workflow editor; Definition save |
| REQ-OFFICE-WORKFLOW-SELECT-004 | Selection API; Settings Workflows tab |
| REQ-OFFICE-WORKFLOW-SELECT-005 | Validation |
| REQ-OFFICE-WORKFLOW-SELECT-006 | Definition save; Single-step write guard; Locking |
| REQ-OFFICE-WORKFLOW-SELECT-007 | Export and import |

## Office marker

**Schema.** `workflows.is_office INTEGER NOT NULL DEFAULT 0` (SQLite and
PostgreSQL through the existing `r.migrate.Apply` path in
`base_migrations.go`). `models.Workflow.IsOffice bool` (`json:"is_office,omitempty"`),
read by `workflowSelectColumns`-style scans in
`internal/task/repository/sqlite/workflow.go`, exposed on `dto.WorkflowDTO`
and the web `Workflow` type.

The marker is backend-authoritative. `workflows.style` stays a frontend-only
UX hint (ADR-0004); copies and imports set `style = 'office'` for presentation,
but no backend path reads `style`.

**Writers.** Only these paths set `is_office = 1`, and nothing clears it:

1. `createWorkflowFromTemplate` when the template declares `office: true`.
   `config/workflows/office-default.yml` gains `office: true`;
   `models.WorkflowTemplate` gains `Office bool` (loader passes it through).
2. Office definition create (copy or blank Office workflow).
3. Import of a portable workflow carrying `office: true`.
4. Selection of an unmarked, task-free workflow.

## Migration

One idempotent migration step after the column is added:

```sql
UPDATE workflows SET is_office = 1
WHERE is_office = 0 AND (
  id IN (SELECT office_workflow_id FROM workspaces WHERE COALESCE(office_workflow_id,'') != '')
  OR (is_system = 1 AND workflow_template_id = 'office-default')
)
```

This reproduces today's membership exactly, so unchanged workspaces see no
difference (AC-001.3). The persistence upgrade fixture and manifest get the new
column, and `go run ./cmd/sqlguard ./internal` covers the new queries.

## Membership readers

Every reader switches from `office_workflow_id` equality to the marker:

Readers treat a workflow as Office when it is marked **or** is its
workspace's selected workflow. Selection always marks the workflow, so the two
sets are equal in production; the union keeps rows written outside the
selection path (fixtures, older tooling) classified correctly.

- `isFromOfficeProjection` adds `EXISTS (SELECT 1 FROM workflows owf WHERE
  owf.id = t.workflow_id AND owf.workspace_id = t.workspace_id AND
  owf.is_office = 1)` beside the existing selected-workflow clause. The
  `project_id` and `routine` clauses are unchanged.
- `Service.GetOfficeWorkflowIDs` reads the task repository's
  `ListOfficeWorkflowIDs(ctx)` (marked workflows union every
  `office_workflow_id`). Callers (`listWorkflows`, `boot_state.go`) are
  unchanged.
- `validateAssigneeTaskIsOffice` accepts a request whose `WorkflowID` names an
  Office workflow of the same workspace (`IsOfficeWorkflowInWorkspace`).
- `handoff.go` refuses any Office workflow of the target workspace as a
  delivery workflow, not only the selected one.

`resolveOfficeWorkflow`, `HasOfficeAdoption`, `create_task_mode.go`, workspace
clone and the web `isOfficeWorkspace` selectors keep reading
`office_workflow_id`: they ask "where do new Office tasks go" or "is this an
Office workspace", which selection answers.

## Validation

Pure function in `internal/workflow/models/office_validation.go`:

```go
type OfficeWorkflowProblem struct {
    Code     string `json:"code"`
    StepName string `json:"step_name,omitempty"`
    Role     string `json:"role,omitempty"`
    Action   string `json:"action,omitempty"`
    Count    int    `json:"count,omitempty"`
}
func ValidateOfficeWorkflow(steps []*WorkflowStep, profileExists func(id string) bool) []OfficeWorkflowProblem
```

| Code | Rule | AC |
| --- | --- | --- |
| `start_step_count` | exactly one `is_start_step` (`Count` = actual) | 005.1 |
| `missing_work_stage` | ≥1 step with `stage_type = work` and `auto_advance_requires_signal` | 005.2 |
| `final_step_not_completing` | highest-position step has `complete_task_on_enter` | 005.3 |
| `completing_step_unreachable` | completing step reachable from the start step over `move_to_step`, `move_to_next` and `move_to_previous` actions in any event; `move_to_next` / `move_to_previous` follow the runtime rule (first step with a strictly higher position, last with a strictly lower one), so tied positions do not count as a move | 005.3 |
| `missing_seat_action` | each `review` / `approval` step has `on_enter` `ensure_participant_seat` and `queue_run_for_each_participant` with the same `role` (`Action` = missing type) | 005.4 |
| `unknown_agent_profile` | every action `agent_profile_id` (`ReviewAgentProfileConfigKey`) resolves | 005.5 |

All failing rules are returned, ordered by step position then code (AC-005.6).
A unit test runs the embedded `office-default` template through it (AC-005.7).
Each code maps to a `workflows:officeWorkflowProblem.<code>` sentence in all
seven web locale catalogs, following the problem-plus-fix shape used by
`review-author-provider-exclusion` warnings.

Errors that are not structural use codes on the same response shape:
`not_in_workspace`, `sync_managed`, `holds_kanban_tasks`,
`requires_office_workspace`, `built_in_read_only`, `selected_delete`,
`step_holds_tasks`, `selected_requires_definition_save`, `invalid_events`
(with `StepName` plus the validator message as `detail`).

## Selection API

The workflow service owns the transaction because it owns step storage and
the step-write locks. Handlers live in
`internal/workflow/handlers/office_workflow_handlers.go`, service methods in
`internal/workflow/service/office_workflow.go`, and the transaction in
`internal/workflow/repository/office_workflow.go`. The task domain supplies
the workspace-manage check (`taskservice.AuthorizeWorkspaceManage`, wired as
`SetWorkspaceManageChecker`) and the change notification
(`PublishOfficeWorkflowChanged`, wired as `SetOfficeWorkflowChangePublisher`):

- `GET /api/v1/workspaces/:id/office-workflow` → `{ selected_workflow_id,
  candidates: [{ id, name, built_in, is_office }] }`. Candidates follow
  AC-004.1: `is_office` and `source = 'manual'`, plus non-Office manual
  workflows with zero tasks that are neither hidden nor system-owned (hidden
  rows are the built-in Kanban templates and Improve Kandev flows, which a
  user cannot see or edit). Requires workspace read.
- `PUT /api/v1/workspaces/:id/office-workflow` `{ workflow_id }`. Requires
  `authz.ScopeWorkspaceManage` (AC-004.5). Refuses a workspace without Office
  (`requires_office_workspace`). Returns the workspace DTO on success, or
  HTTP 422 `{ code: "office_workflow_invalid", problems: [...] }`.

Repository method `SelectOfficeWorkflow(ctx, workspaceID, workflowID,
check OfficeWorkflowCheck)` runs in one transaction under the locking
order below: lock the workspace row, lock the workflow row, check workspace
ownership, `source`, and (if `is_office = 0`) zero tasks; load steps; run
validation; then set `workspaces.office_workflow_id` and `workflows.is_office = 1`.
After commit the service publishes `events.WorkspaceUpdated` and
`events.WorkflowUpdated` so the boot store, Kanban board and settings tab
refresh (AC-004.3). `stampWorkspaceOfficeWorkflow` keeps filling only an
empty column.

## Definition save

Office copies are saved as a whole, never step by step:

- `POST /api/v1/workspaces/:id/office-workflows` creates an Office copy.
- `PUT /api/v1/workflows/:id/definition` replaces an Office copy.

Body: `{ expected_updated_at?, expected_steps?, name, description, prompt,
agent_profile_id, steps: [{ id, name, position, color, prompt, stage_type,
is_start_step, events, ...every persisted step field... }] }`. An `id` that
names an existing step of the workflow updates it; any other `id` is a client
key (the editor's `temp-step-*` ID) for a new step. Step references in
`events`, `pull_from_step_id` and session targets may use either. A repeated
`id` is refused (`invalid_events`), so validation and the write always see the
same step set. Positions are taken from the order of `steps`.

`expected_updated_at` is the workflow version the editor loaded and
`expected_steps` maps every step it loaded to that step's `updated_at`. Both
are compared inside the transaction; a different workflow version or stored
step set returns HTTP 409 `{ code: "workflow_changed" }`. The step fence
catches writers that change steps without touching the workflow row, such as
the Office > Routing toggle and single-step writes to an unselected copy.
Versions are written at microsecond precision so a returned version can be
sent back unchanged.

Service `SaveOfficeWorkflowDefinition` (`internal/workflow/service/office_definition.go`):

1. Authorize as workflow creation does (`authz.ScopeRepositoryManage`). Refuse
   `is_system` (`built_in_read_only`), non-Office targets, and workspaces
   without Office.
2. Assign UUIDs to new keys and rewrite references; check each step
   (`ValidateWorkflowStep`, known actions, references inside the workflow),
   then the workflow-wide session-target rules (`validateWorkflowSessionTargets`)
   and pull chains (no self or cyclic `pull_from_step_id`), all as
   `invalid_events` (AC-003.3).
3. In one transaction, under the locking order: compare `expected_updated_at`
   and `expected_steps` (mismatch → HTTP 409 `workflow_changed`, the editor
   keeps the draft and asks for a reload); refuse removed steps that hold tasks (`step_holds_tasks`,
   AC-006.3); when the workflow is the workspace's selected Office workflow,
   run `ValidateOfficeWorkflow` on the resulting steps and refuse on any
   problem (AC-006.1); then update the workflow row, update kept steps, insert
   new steps, clear references to and delete removed steps, and write
   positions with the two-phase approach in `repository/reorder.go`.
4. After commit publish the existing workflow and step created/updated/deleted
   events.

Because all writes share one transaction, other readers see the previous or
the saved workflow, never a mix (AC-006.2).

## Single-step write guard

`internal/workflow/service` step mutations (`CreateStep`, `UpdateStep`,
`DeleteStep`, `ReorderSteps`, `CreateStepsFromTemplate`) ask the task-domain
policy `OfficeStepWriteRefusal` (wired as `SetOfficeStepWritePolicy`) about the
owning workflow: when it is a built-in Office workflow they return
`built_in_read_only`; when it is the selected Office workflow they return
`selected_requires_definition_save` (HTTP 409, AC-006.4). Workflow metadata
PATCH stays allowed.

That early check reads outside the write. To close the gap with a selection
that commits in between, a guarded write carries
`repository.WithOfficeStepGuard(ctx)`, and the repository repeats the check
inside the create, update, delete and reorder transactions after their locks
(`recheckOfficeStepGuard`), returning `OfficeStepWriteRefusedError`, which
handlers map to the same 409. Writes without the marker (internal callers,
workflow sync, the Routing toggle) are not affected. Deleting a step is two
writes (clearing references, then the delete); a selection that commits
between them makes the delete refuse after references are already cleared,
as the pre-existing non-atomic step delete already allows. `SetSeatAuthorExclusion` writes through
`UpdateStepEventsIfUnchanged`, not these methods, and only toggles
`exclude_author_provider`, which no validation rule reads, so Office > Routing
keeps working.

## Locking

Selection and definition save both lock the workspace row first, then the
workflow row (`SELECT ... FOR UPDATE` on PostgreSQL; SQLite serializes writers
at transaction start, as `lockWorkflowRowForStepWrite` documents). A save
therefore validates against the selection that is current when it commits,
and a selection validates the steps that are current when it commits
(AC-006.5). Single-step writes take the workspace lock (create, update,
delete) or the workflow row lock (reorder) and re-read the selection inside
the transaction, in the same order. Test: `office_workflow_race_test.go`
runs selection and an invalidating save in both orders and concurrently, on
SQLite and PostgreSQL, and requires exactly one to succeed and the other to
be refused with its named problem.

Task creation and moves resolve their destination step before their own
transaction. A definition save that removes an unoccupied step in that window
can leave the new task on a removed step ID; it then appears in the existing
**Needs Reassignment** column, the recovery path the board already uses for a
deleted step. Task rows do not reference steps with a foreign key, and test
fixtures rely on that, so this design does not add a check there.

## Deletion guard

`Service.DeleteWorkflow` refuses `is_system` Office workflows
(`built_in_read_only`) and the workspace's selected Office workflow
(`selected_delete`), AC-002.5. The repository delete repeats the selection
check inside its transaction (`ErrOfficeWorkflowSelected`), so a selection
that commits after the early check cannot leave a dangling
`office_workflow_id`. The settings card disables **Delete** with the named
reason for the built-in and the selected workflow, and **Migrate & Delete**
re-reads the selection before moving any task. Other deletions keep their
existing rules.

## Export and import

- `models.WorkflowPortable` gains `Office bool` (`yaml:"office,omitempty"`).
  Export sets it from `IsOffice`; stage types, events and
  `ConvertReviewProfileToPortable` already round-trip (AC-007.1). Each Office
  card's **Export** button exports that workflow by ID.
- Import (`httpImportWorkflows` and preview): when any workflow has
  `office: true` and the workspace has no Office workflow, the whole import is
  refused before writing with `requires_office_workspace` (AC-007.3).
  Otherwise Office workflows are created with `is_office = 1`,
  `style = 'office'` and the existing name and profile selection (AC-007.2,
  007.4).
- `docs/workflow-import-export.md` drops the stale note that `stage_type` is
  not portable and documents `office`.

## Settings Workflows tab

`loadWorkspaceWorkflowsRoute` (`src/settings-routes.workspace-data.tsx`) lists
with `includeHidden: true` and `exclude_office=false`
(`listWorkflows` in `lib/api/domains/kanban-api.ts` gains `excludeOffice`).
`useWorkspaceWorkflowsPage` splits the result: Kanban list = `!is_office &&
style !== "office" && !hidden`; Office section = `is_office`. The Kanban store
drops a workflow whose `workflow.updated` payload carries `is_office: true`
and ignores `workflow.created` for Office workflows.

New `app/settings/workspace/office-workflows-section.tsx`, rendered only when
`workspace.office_workflow_id` is set (AC-002.4):

- **Picker.** `Select` bound to `GET .../office-workflow`; choosing calls the
  PUT. Disabled unless `hasScope(workspace.scopes, SCOPE.workspaceManage)`.
  It is a `SettingsRow` in one bordered `SettingsGroupContent` under the
  section heading, the preference-group pattern from
  [Layout in DESIGN.md](../../../../DESIGN.md#settings). A 422 renders the
  localized problem list in an inline `Alert` under that group and keeps the
  previous value.
- **List.** One `WorkflowCard` per Office workflow. The built-in shows a
  **Built-in** badge, read-only, with Duplicate and Export. Copies are
  editable, with Duplicate, Export and Delete. The selected workflow shows a
  **Selected** badge.
- **Duplicate** reuses `createWorkflowDuplication`; the draft keeps
  `is_office: true`, lands in the Office section, and shows the
  "won't receive built-in updates" notice when the source `is_system`
  (AC-002.3).
- **Save.** `persistWorkflowDraft` (`components/settings/workflow-card-actions.ts`)
  branches on `is_office`: one POST or PUT definition call instead of the
  per-step calls, sending the loaded versions. A refusal or version conflict
  shows the localized reasons in an error toast (the floating save bar only
  says the save failed); unsaved edits stay.
- **Import** stays the tab-level action; Office files land in the Office
  section after reload.

Phone (≤390px): the picker row stacks its label above a full-width control
with a 44px target, above the list; badges wrap below
the workflow name; card actions use the existing overflow menu; the problem
list wraps without horizontal scroll (AC-002.6, AC-004.6).

## Office workflow editor

`WorkflowCard` / `StepConfigPanel` (`components/settings/workflow-pipeline-editor-panels.tsx`)
take the Office mode from `workflow.is_office`:

- `StepHoldRow` is replaced by a **Stage type** `Select` (work, review,
  approval, custom, hold) bound to `stage_type` (AC-003.1).
- New `workflow-step-office-events.tsx` renders an **Office events** section:
  a monospace `Textarea` (as in `workflow-export-dialog.tsx`) holding the
  step's `events` serialized with the `yaml` package, step IDs shown as step
  names. Parsing happens on change. Valid text updates the step's `events`;
  text that does not parse is kept on the step as the client-only
  `office_events_draft`, shows a located error, and blocks the route-level
  Save (`officeEventsMustBeFixed`) until fixed, so nothing is written with
  stale events. The shown text follows the stored events when a step rename
  or a discard makes the last valid text stale. The client checks YAML
  syntax, triggers, action types and step references; the backend re-checks
  action types (`models.ValidateKnownStepActions`) and references. Action
  config keys pass through unchecked, as elsewhere in the workflow editor;
  the runtime ignores unknown keys.
- Built-in Office workflows render with the existing `readOnly` prop
  (AC-003.4).

## Failure and recovery

- Validation and guard refusals write nothing and return 422 or 409 with
  problem codes. The editor keeps the draft; the picker keeps its value.
- A save that loses a version race returns 409 `workflow_changed`; the editor
  keeps the draft and tells the user to reload, then reapply the change.
- A failed definition create leaves no workflow (single transaction), so the
  per-step partial-save cleanup is not needed for Office copies.

## Observability

Selection logs `workspace_id`, previous and new `workflow_id` at info.
Refusals log the problem codes at debug. No new metrics.

## Related ADRs

- [ADR-0004 Task model unification](../../../decisions/0004-task-model-unification.md)
- [ADR 2026-11-24 Office workflow marker](../../../decisions/2026-11-24-office-workflow-marker.md)
