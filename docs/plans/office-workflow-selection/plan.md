---
created: 2026-11-24
status: implemented
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-001
  - REQ-OFFICE-WORKFLOW-SELECT-002
  - REQ-OFFICE-WORKFLOW-SELECT-003
  - REQ-OFFICE-WORKFLOW-SELECT-004
  - REQ-OFFICE-WORKFLOW-SELECT-005
  - REQ-OFFICE-WORKFLOW-SELECT-006
  - REQ-OFFICE-WORKFLOW-SELECT-007
system_design:
  - ../../specs/office/system-design/office-workflow-selection.md
legacy_specs: []
---

# Implementation Plan: Office Workflow Selection

## Overview

Let a workspace manager copy, edit, select, export and import its Office
workflow (GitHub issue #5347). Delivery runs bottom-up so each layer ships on a
working product:

1. The Office marker replaces `office_workflow_id` equality for membership.
   It must come first because a selection API without it would reclassify
   tasks in flight.
2. The pure validation function, independent of storage.
3. Selection API, whole-workflow definition save, and export/import, which all
   need the marker and validation.
4. Web: the Office section of Settings > Workflows, then the Office editor
   controls.
5. One end-to-end flow across both, plus public docs.

## Scope

### In scope

- `workflows.is_office` marker, backfill, and every membership reader.
- `ValidateOfficeWorkflow` with named problem codes.
- `GET`/`PUT /api/v1/workspaces/:id/office-workflow`; deletion guard.
- Whole-workflow Office definition create and replace; single-step write
  guard for built-in and selected Office workflows.
- Portable `office` marker on export and import; Office-workspace check.
- Office workflows section in Settings > Workflows: picker, list, duplicate,
  definition save, phone layout.
- Stage-type selector and Office events YAML editor for Office copies.
- Locales for all new copy in all seven catalogs.
- `docs/workflow-import-export.md`, `docs/public/workflow-import-export.md`,
  `docs/public/office.md` updates.

### Out of scope

- Migrating in-flight tasks to a new Office workflow.
- Structured per-trigger forms; moving **Review on a different provider** into
  the editor.
- Workflow sync for Office workflows; MCP or plugin selection tools.
- Template upgrades for Office copies.

## Technical approach

**Marker and readers (task 01).** Migration in
`internal/task/repository/sqlite/base_migrations.go` adds
`workflows.is_office` and backfills from `workspaces.office_workflow_id` and
system `office-default` rows. `models.Workflow.IsOffice`, `dto.WorkflowDTO`,
workflow scans in `repository/sqlite/workflow.go`. `office: true` in
`config/workflows/office-default.yml`, `WorkflowTemplate.Office`, and
`createWorkflowFromTemplate` writes it. Readers:
`isFromOfficeProjection`, `GetOfficeWorkflowIDs` → `ListOfficeWorkflowIDs`,
`validateAssigneeTaskIsOffice`, `office/runtime/handoff.go`. Persistence upgrade
fixture and `requiredstores` conformance updated.

**Validation (task 02).** New `internal/workflow/models/office_validation.go`
with `ValidateOfficeWorkflow` and `OfficeWorkflowProblem` codes from the design.
Pure, so it is shared by selection, definition save and tests.

**Selection (task 03).** `internal/task/service/service_office_workflow.go`
plus a repository transaction `SelectOfficeWorkflow` that locks workspace then
workflow, checks eligibility, validates and writes both columns. Handlers in
`internal/task/handlers/workspace_handlers.go`. `DeleteWorkflow` guard.
Publishes `events.WorkspaceUpdated` and `events.WorkflowUpdated`.

**Definition save (task 04).** `internal/workflow/service/office_definition.go`
and a repository transaction that takes the same lock order, maps client keys,
validates, refuses removed steps with tasks, and writes metadata plus steps.
Routes `POST /api/v1/workspaces/:id/office-workflows` and
`PUT /api/v1/workflows/:id/definition` in `internal/workflow/handlers`.
Guards in `CreateStep`, `UpdateStep`, `DeleteStep`, `ReorderSteps`,
`CreateStepsFromTemplate`.

**Export/import (task 05).** `WorkflowPortable.Office`; export sets it; import
refuses Office files into non-Office workspaces and creates `is_office`
workflows otherwise.

**Settings tab (task 06).** `loadWorkspaceWorkflowsRoute` loads hidden and
Office workflows; `useWorkspaceWorkflowsPage` splits Kanban and Office lists by
`is_office`. New `office-workflows-section.tsx`, `use-office-workflow-selection.ts`,
API client `lib/api/domains/office-workflow-api.ts`. `persistWorkflowDraft`
branches to the definition endpoints for `is_office` drafts.

**Editor (task 07).** `StepConfigPanel` stage-type `Select` for Office
workflows; new `workflow-step-office-events.tsx` with YAML serialize/parse via
the existing `yaml` dependency and name-based step references.

This is the simplest design that meets the requirements: one column instead of
a membership history, one pure validator shared by every write, and one
whole-workflow save path instead of making every single-step endpoint
validation-aware. The atomic save exists because AC-006.1 and AC-006.2 forbid
the intermediate states that per-step saves produce.

## ASCII UI preview

### UI-01: Settings > Workflows, Office workspace (desktop)

Entry: Settings > Workspaces > *workspace* > Workflows. Office section above
the existing Kanban list. Structural requirements: picker first, then Office
workflow cards; built-in labelled and read-only; selected labelled. Spacing is
illustrative.

```text
+---------------------------------------------------------------------------+
| Workflows                                  [Import] [Export all] [+ New]  |
|---------------------------------------------------------------------------|
| Office workflows                                                          |
| New Office tasks use the selected workflow. Tasks already created keep    |
| theirs.                                                                   |
|                                                                           |
| Office workflow  [ Office Default (copy)                          v ]     |
|                                                                           |
| +-----------------------------------------------------------------------+ |
| | Office Default   [Built-in] [Read-only]            [Duplicate] [...]  | |
| |  Backlog > Work > Review > Approval > Done                            | |
| +-----------------------------------------------------------------------+ |
| +-----------------------------------------------------------------------+ |
| | Office Default (copy)   [Selected]        [Duplicate] [Export] [...]  | |
| |  Backlog > Work > Verify > Review > Approval > Done                   | |
| +-----------------------------------------------------------------------+ |
|---------------------------------------------------------------------------|
| Kanban workflows                                                          |
|  (existing list, unchanged; Office workflows never appear here)           |
+---------------------------------------------------------------------------+
```

### UI-02: Selection refused (desktop and phone)

Shown under the picker after a refused choice; the picker keeps the previous
value. AC-004.4, AC-005.6.

```text
Office workflow  [ Office Default (copy)                          v ]
+-------------------------------------------------------------------------+
| ! "Lean office" can't be the Office workflow                           |
|  - Step "Ship" is the last step but doesn't complete the task. Turn on |
|    "Complete task when entering this step" or move it before "Done".   |
|  - Step "Verify" is a Review step without a reviewer seat. Add an      |
|    ensure_participant_seat action for role reviewer to its on_enter.   |
+-------------------------------------------------------------------------+
```

### UI-03: Duplicate draft of the built-in (desktop)

AC-002.3.

```text
+-----------------------------------------------------------------------+
| Office Default (copy)   [Unsaved]                                     |
| i This copy won't receive future updates to the built-in Office        |
|   Default workflow.                                                    |
|  Backlog > Work > Review > Approval > Done                            |
+-----------------------------------------------------------------------+
```

### UI-04: Office step editor (desktop)

Step panel of an Office copy. Stage type replaces the hold checkbox; Office
events is an advanced section. AC-003.1 to 003.3.

```text
+-- Step: Verify ---------------------------------------------------------+
| Name [Verify          ]  Color [o]                                      |
| Stage type [ Review                  v ]                                |
| [x] Allow manual move   [ ] Requires completion signal  ...             |
|                                                                         |
| v Office events (advanced)                                              |
| +---------------------------------------------------------------------+ |
| | on_enter:                                                           | |
| |   - type: run_code_review                                           | |
| |     config:                                                         | |
| |       agent_profile_id: 7f3c...                                     | |
| |   - type: ensure_participant_seat                                   | |
| |     config: { role: reviewer }                                      | |
| | on_turn_complete:                                                   | |
| |   - type: move_to_step                                              | |
| |     config: { step_id: Review }   # step name; saved as the step ID | |
| +---------------------------------------------------------------------+ |
| x Line 7: unknown action "ensure_seat" in on_enter.                     |
+-------------------------------------------------------------------------+
```

### UI-05: Phone (390px)

Composition differs: picker full width; badges wrap under the name; card
actions collapse into the existing overflow menu; YAML editor full width with
horizontal scroll inside the textarea only.

```text
+--------------------------------+
| Office workflows               |
| New Office tasks use the       |
| selected workflow.             |
| Office workflow                |
| [ Office Default (copy)    v ] |
| +----------------------------+ |
| | Office Default        [..] | |
| | [Built-in] [Read-only]     | |
| | Backlog > Work > ...       | |
| +----------------------------+ |
| +----------------------------+ |
| | Office Default (copy) [..] | |
| | [Selected]                 | |
| | Backlog > Work > Verify... | |
| +----------------------------+ |
| Kanban workflows               |
+--------------------------------+
```

## Tests

| AC | Evidence |
| --- | --- |
| 001.1 | `internal/task/repository/sqlite/office_workflow_marker_test.go` `TestIsFromOfficeAfterSelectionChange` |
| 001.2 | `internal/task/handlers/workflow_handlers_office_test.go` `TestListWorkflowsExcludesAllOfficeWorkflows`; `internal/backendapp/boot_state_office_workflows_test.go` |
| 001.3 | `office_workflow_marker_test.go` `TestBackfillMarksCurrentOfficeWorkflowsOnly` |
| 002.5 | `internal/task/service/service_office_workflow_test.go` `TestDeleteWorkflowRefusesBuiltInAndSelectedOffice` |
| 003.3 | `internal/workflow/service/office_definition_test.go` `TestDefinitionSaveRejectsInvalidEvents`; `components/settings/workflow-step-office-events.test.tsx` |
| 004.1–004.5 | `service_office_workflow_test.go` (candidates, eligibility, scope); `app/settings/workspace/use-office-workflow-selection.test.ts` |
| 005.1–005.7 | `internal/workflow/models/office_validation_test.go` table test incl. `TestOfficeDefaultTemplatePasses` |
| 006.1–006.4 | `office_definition_test.go` (`TestSelectedSaveRefusedWhenInvalid`, `TestSaveIsAtomic`, `TestSaveRefusesRemovingStepWithTasks`); `internal/workflow/service/office_step_guard_test.go` |
| 006.5 | `internal/workflow/service/office_workflow_race_test.go` `TestSQLiteSelectionAndSaveSerialize`, `TestPostgresSelectionAndSaveSerialize` |
| 007.1–007.4 | `internal/workflow/models/export_office_test.go`; `internal/workflow/service/import_office_test.go` |
| 002.1–002.4 | `app/settings/workspace/office-workflows-section.test.tsx`; `components/settings/workflow-card-actions-office.test.ts` |
| 003.1, 003.4 | `components/settings/workflow-pipeline-editor-office.test.tsx` |

## E2E tests

| Flow | ACs | File and project |
| --- | --- | --- |
| Duplicate built-in, add Verify review step via Office events, select, create Office task, task lands on the copy, old task unchanged, workflow absent from Kanban board | 001.1, 001.2, 002.2, 002.3, 003.1, 003.2, 004.2, 004.3 | `apps/web/e2e/tests/office/office-workflow-selection.spec.ts`, `chromium` |
| Refused selection lists named problems; invalid save of selected workflow refused | 004.4, 005.6, 006.1 | same file, `chromium` |
| Export the copy, import into a second Office workspace, select it | 007.1, 007.2 | same file, `chromium` |
| Phone layout of section, picker, refusal, duplicate notice | 002.6, 004.6 | `apps/web/e2e/tests/office/mobile-office-workflow-selection.spec.ts`, `mobile-chrome` |

## Work orders

- [x] [Task 01: Office workflow marker](task-01-office-workflow-marker.md)
- [x] [Task 02: Office workflow validation](task-02-office-workflow-validation.md)
- [x] [Task 03: Office workflow selection API](task-03-selection-api.md)
- [x] [Task 04: Whole-workflow Office definition save](task-04-definition-save.md)
- [x] [Task 05: Office export and import](task-05-export-import.md)
- [x] [Task 06: Office workflows section in settings](task-06-settings-office-section.md)
- [x] [Task 07: Office step editor controls](task-07-office-step-editor.md)
- [x] [Task 08: End-to-end flow and public docs](task-08-e2e-and-docs.md)

Dependency order: 01 and 02 (parallel-safe) → 03 and 05 (parallel-safe) →
04 → 06 → 07 → 08.

## Verification results

- Backend: `make -C apps/backend test` passes except the known host-environment failures `TestManagedNPMRuntimeLaunchIgnoresWorkspaceNpmrc` (npm) and `TestCreateWorktree_RefreshLockPolicy` (git), outside the changed code. PostgreSQL suites for the task and workflow repositories, workflow service and store conformance pass against a local instance.
- `make fmt`: no changes. `make typecheck`: pass. `make lint` (backend, web, harness, specs, architecture): pass.
- Web: `make test-web`: 3511 of 3512 files pass; the one failure is `e2e/helpers/seed-repository-origin.test.ts`, blocked by the host's git push identity hook (environment, unrelated). `i18n:check` and `i18n:ratchet` pass.
- E2E: Office workflow selection (chromium 3, mobile-chrome 1) and the adjusted Office/workflow specs pass.
- Docs: `list-docs.py validate`, `lint-spec-files.py --all`, `validate-public-docs.mjs` pass.

## Risks

- **Membership readers missed.** Any remaining `office_workflow_id` equality
  check would misclassify tasks after a switch. Task 01 greps for every
  `OfficeWorkflowID ==` and `office_workflow_id =` comparison.
- **Atomic step rewrite.** Rewriting positions and deleting steps in one
  transaction must respect step uniqueness and reference clearing. Reuse
  `repository/reorder.go`'s two-phase position write and
  `ClearStepReferences`.
- **YAML step references.** Showing step names instead of IDs needs a
  reversible mapping; duplicate step names must be refused in the editor.
- **Agent profile IDs in events.** `run_code_review` profile IDs are
  install-specific; export/import relies on the existing portable profile
  conversion, and validation catches dangling IDs.
- **Upgrade fixture.** The new column changes schema history; the explicit-tag
  upgrade fixture and manifest must be updated in task 01.
