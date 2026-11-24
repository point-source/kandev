---
status: active
system: office
created: 2026-11-24
owners:
  - kandev
---

# Office Workflow Selection Requirements

## Overview

Every Office workspace runs the built-in **Office Default** workflow (Backlog,
Work, Review, Approval, Done). A workspace manager cannot change that process:
no surface selects a different Office workflow, and the Office workflow is
missing from the workflow editor, from Settings > Workflows, and from export.

This capability lets a workspace manager copy an Office workflow, edit the copy
in the existing workflow editor (stage types, Office triggers and actions),
select it as the workspace's Office workflow, and move it between workspaces
through the portable workflow format. Office routing, seats, quorum and stall
detection already key off stage types and workflow actions rather than step
names, so a valid custom Office workflow drives Office the same way the
built-in one does.

The Office system owns this contract because it owns which workflow Office
tasks run on and which tasks are Office tasks. Generic workflow primitives
(steps, events, duplication of Kanban workflows, the portable format) remain
owned by the [task system](../../tasks/README.md); this document references
them and does not restate them.

GitHub issue: #5347.

## Terminology

- **Office workflow:** a workflow that Office owns. Its tasks are Office tasks
  and it never appears on the Kanban board. The built-in Office Default, every
  copy made from an Office workflow, every Office import, and every workflow
  that has ever been selected are Office workflows. A workflow stays an Office
  workflow until it is deleted.
- **Selected Office workflow:** the one Office workflow in a workspace that new
  Office tasks are created on.
- **Built-in Office workflow:** the system-maintained Office Default. It is
  read-only and receives template upgrades.
- **Office copy:** a user-owned Office workflow. It is editable and does not
  receive template upgrades.
- **Completing step:** the workflow's final step with **Complete task when
  entering this step** enabled (`AC-TASKS-COMPLETION-001.2`).
- **Seat actions:** a step's `on_enter` `ensure_participant_seat` action and
  the `queue_run_for_each_participant` action for the same role.
- **Workspace manager:** a user allowed to manage the workspace's settings.

## Requirements

### REQ-OFFICE-WORKFLOW-SELECT-001: Office workflow membership

**Intent:** Tasks keep their Office behavior and Office workflows stay off the
Kanban board when the selected Office workflow changes.

#### Acceptance criteria

- **AC-OFFICE-WORKFLOW-SELECT-001.1:** When a workspace selects a different
  Office workflow, every task created before the change shall keep its
  workflow, step, participant seats, recorded decisions and runs, and shall
  remain an Office task in Office task lists, the Inbox, Office routing and the
  task page's Office view.
- **AC-OFFICE-WORKFLOW-SELECT-001.2:** Office workflows, selected or not, shall
  not appear as Kanban board swimlanes, in Kanban workflow pickers, or as
  workflow choices for new Kanban tasks.
- **AC-OFFICE-WORKFLOW-SELECT-001.3:** When Kandev upgrades a workspace that
  never changes the setting, its existing Office workflow and Office tasks
  shall behave exactly as before the upgrade, and no workflow shall be added to
  or removed from its Kanban board.

### REQ-OFFICE-WORKFLOW-SELECT-002: Office workflows in workflow settings

**Intent:** A workspace manager can see and copy Office workflows in the
existing Settings > Workflows tab.

**User story:** As a workspace manager, I want to duplicate the built-in Office
workflow, so that I can change my workspace's Office process without starting
from nothing.

#### Acceptance criteria

- **AC-OFFICE-WORKFLOW-SELECT-002.1:** When a user opens Settings > Workflows
  for an Office workspace, the tab shall show an **Office workflows** section
  that lists the built-in Office workflow, labelled built-in and read-only, and
  every Office copy. Kanban workflows shall remain in their existing list and
  Office workflows shall not appear in it.
- **AC-OFFICE-WORKFLOW-SELECT-002.2:** When a workspace manager chooses
  **Duplicate** on an Office workflow and saves, Kandev shall create an Office
  copy in the Office workflows section. The copy shall carry every step's
  configuration, including stage type, completion-signal policy, Office
  triggers and actions, and agent profiles. It shall get new workflow and step
  identities, with every step reference remapped to the copied steps, and no
  tasks, history or template identity. Duplication follows the existing draft
  and Save behavior of `REQ-TASKS-WORKFLOW-DUPLICATION-001`.
- **AC-OFFICE-WORKFLOW-SELECT-002.3:** When the source of a duplicate is the
  built-in Office workflow, the draft shall show a notice that the copy will
  not receive future updates to the built-in workflow.
- **AC-OFFICE-WORKFLOW-SELECT-002.4:** A workspace without Office enabled shall
  show no Office workflows section.
- **AC-OFFICE-WORKFLOW-SELECT-002.5:** When a user tries to delete the built-in
  Office workflow or the selected Office workflow, Kandev shall refuse and name
  the reason. Deleting an unselected Office copy follows existing workflow
  deletion rules.
- **AC-OFFICE-WORKFLOW-SELECT-002.6:** At a 390px touch viewport, the Office
  workflows section, its actions and the duplicate notice shall be usable
  without document-level horizontal overflow.

### REQ-OFFICE-WORKFLOW-SELECT-003: Editing Office copies

**Intent:** The existing workflow editor can change everything Office needs.

#### Acceptance criteria

- **AC-OFFICE-WORKFLOW-SELECT-003.1:** When a workspace manager edits an Office
  copy, each step shall offer a stage-type selector with Work, Review,
  Approval, Custom and Hold.
- **AC-OFFICE-WORKFLOW-SELECT-003.2:** Each step of an Office copy shall offer
  an advanced **Office events** editor that shows the step's events as YAML,
  covering every trigger (`on_enter`, `on_comment`, `on_blocker_resolved`,
  `on_children_completed`, `on_agent_error`, `on_turn_complete` and the other
  supported step events) and every supported action and action setting.
- **AC-OFFICE-WORKFLOW-SELECT-003.3:** When the Office events YAML is not valid
  YAML, or names an unknown trigger or action, or a step that does not exist,
  Save shall be refused with a message that names the problem and its
  location, and nothing shall be written. Action settings are passed to the
  runtime as written.
- **AC-OFFICE-WORKFLOW-SELECT-003.4:** The built-in Office workflow shall open
  read-only in the editor, with **Duplicate** as the way to change it.

### REQ-OFFICE-WORKFLOW-SELECT-004: Choosing the Office workflow

**Intent:** A workspace manager sets the workspace's Office workflow without
editing the database.

#### Acceptance criteria

- **AC-OFFICE-WORKFLOW-SELECT-004.1:** The Office workflows section shall show
  an **Office workflow** picker with the current selection. Its candidates
  shall be the built-in Office workflow, every Office copy that is not
  sync-managed, and every workspace workflow that has no tasks, is not
  sync-managed, and is neither hidden nor system-owned.
- **AC-OFFICE-WORKFLOW-SELECT-004.2:** When a workspace manager chooses a
  candidate that passes `REQ-OFFICE-WORKFLOW-SELECT-005`, Kandev shall make it
  the selected Office workflow, and every Office task created afterwards by a
  user, an agent or onboarding shall start on that workflow's start step.
  Routine runs keep starting on the workspace's dedicated Routine workflow.
- **AC-OFFICE-WORKFLOW-SELECT-004.3:** When the chosen workflow is a workspace
  workflow without tasks that was not yet an Office workflow, selecting it
  shall make it an Office workflow, and it shall move from the Kanban list to
  the Office workflows section and leave the Kanban board.
- **AC-OFFICE-WORKFLOW-SELECT-004.4:** When a request selects a workflow from
  another workspace, a sync-managed workflow, or a workflow that is not an
  Office workflow and holds tasks, Kandev shall refuse it with a named reason
  and keep the current selection.
- **AC-OFFICE-WORKFLOW-SELECT-004.5:** A user who is not a workspace manager
  shall see the picker disabled, and a direct selection request from that user
  shall be refused without changing the selection.
- **AC-OFFICE-WORKFLOW-SELECT-004.6:** At a 390px touch viewport, the picker,
  its candidates and any refusal reasons shall be usable without
  document-level horizontal overflow.

### REQ-OFFICE-WORKFLOW-SELECT-005: Office workflow validation

**Intent:** Kandev refuses a workflow Office cannot drive and says what is
missing.

#### Acceptance criteria

- **AC-OFFICE-WORKFLOW-SELECT-005.1:** When a workflow does not have exactly
  one start step, validation shall fail and say how many start steps it has.
- **AC-OFFICE-WORKFLOW-SELECT-005.2:** When no step has the Work stage type
  with **Requires completion signal** enabled, validation shall fail with that
  reason.
- **AC-OFFICE-WORKFLOW-SELECT-005.3:** When the final step is not a completing
  step, or the completing step cannot be reached from the start step through
  the workflow's step-move actions, validation shall fail and name the step.
- **AC-OFFICE-WORKFLOW-SELECT-005.4:** When a Review or Approval step lacks its
  seat actions, validation shall fail and name the step and the missing
  action.
- **AC-OFFICE-WORKFLOW-SELECT-005.5:** When an action names an agent profile
  that does not exist in the installation, validation shall fail and name the
  step and action.
- **AC-OFFICE-WORKFLOW-SELECT-005.6:** When several rules fail, the refusal
  shall list every failure, each in a localized sentence that states the
  problem and the fix.
- **AC-OFFICE-WORKFLOW-SELECT-005.7:** The built-in Office workflow shall pass
  validation.

### REQ-OFFICE-WORKFLOW-SELECT-006: Guarded edits to the selected workflow

**Intent:** Edits never leave the selected Office workflow in a shape Office
cannot drive, even for a moment.

#### Acceptance criteria

- **AC-OFFICE-WORKFLOW-SELECT-006.1:** When a workspace manager saves changes
  to the selected Office workflow, Kandev shall validate the whole resulting
  workflow before writing. If validation fails, the save shall be refused with
  the reasons of `REQ-OFFICE-WORKFLOW-SELECT-005`, nothing shall be written,
  and the editor shall keep the unsaved changes.
- **AC-OFFICE-WORKFLOW-SELECT-006.2:** A successful save of an Office workflow
  shall apply all of its metadata and step changes together. A task created or
  moved during the save shall see either the whole previous workflow or the
  whole saved workflow.
- **AC-OFFICE-WORKFLOW-SELECT-006.3:** When a save would remove a step that
  holds tasks, the save shall be refused and name the step.
- **AC-OFFICE-WORKFLOW-SELECT-006.4:** When a single-step write (create,
  update, delete or reorder one step) targets the selected Office workflow
  outside a whole-workflow save, Kandev shall refuse it with a named reason.
  The **Review on a different provider** control on Office > Routing shall
  keep working.
- **AC-OFFICE-WORKFLOW-SELECT-006.5:** When a selection and a save of the same
  workspace happen at the same time, the saved workflow shall not become the
  selected Office workflow in a state that fails validation.

### REQ-OFFICE-WORKFLOW-SELECT-007: Office workflow export and import

**Intent:** An Office process can live in version control and move between
workspaces and installs.

#### Acceptance criteria

- **AC-OFFICE-WORKFLOW-SELECT-007.1:** When a user exports an Office workflow,
  the portable file shall include stage types, Office triggers and actions,
  portable agent profile references, and a marker that the workflow is an
  Office workflow. It shall not include participants, decisions, runs or
  tasks.
- **AC-OFFICE-WORKFLOW-SELECT-007.2:** When a user imports a file containing an
  Office workflow into an Office workspace, Kandev shall create an Office copy
  that appears in the Office workflows section and not on the Kanban board,
  and that copy shall be selectable once it passes validation.
- **AC-OFFICE-WORKFLOW-SELECT-007.3:** When a file containing an Office
  workflow is imported into a workspace without Office, Kandev shall refuse
  that workflow with a named reason before writing anything.
- **AC-OFFICE-WORKFLOW-SELECT-007.4:** When an exported Office workflow is
  imported back, the imported copy shall have the same steps, stage types,
  triggers, actions and settings as the source, with agent profiles resolved
  by the existing import profile selection.

## Out of scope

- Migrating tasks already in flight to the newly selected workflow.
- Structured forms for each Office trigger and action; the Office events YAML
  editor is the editing surface.
- Moving **Review on a different provider** from Office > Routing into the
  editor.
- Selecting or editing an Office workflow through workflow sync; sync-managed
  workflows cannot be selected.
- Applying future built-in template fixes to Office copies.
- Selecting an Office workflow through MCP or plugin tools.
