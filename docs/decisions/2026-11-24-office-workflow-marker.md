# ADR-2026-11-24-office-workflow-marker: Office workflow membership is a durable per-workflow marker

**Status:** accepted
**Date:** 2026-11-24
**Area:** backend

## Context

A workspace's Office workflow is stored in `workspaces.office_workflow_id`.
The same column also decides which tasks are Office tasks
(`isFromOfficeProjection`) and which workflows are hidden from the Kanban board
(`GetOfficeWorkflowIDs`). GitHub issue #5347 lets a workspace manager select a
different Office workflow. With one column doing both jobs, a switch would turn
existing tasks on the previous workflow into Kanban tasks, drop their Inbox,
routing and seat behavior, and put the previous workflow back on the board.

`workflows.style` already has an `office` value, but ADR-0004 makes `style` a
frontend-only presentation hint that backend code must not branch on.

## Decision

Separate membership from selection.

- `workflows.is_office` is a backend-authoritative marker. A workflow becomes
  an Office workflow when it is materialized from an Office template, created
  as an Office copy, imported from a portable file marked `office: true`, or
  selected as a workspace's Office workflow. Nothing clears the marker; it
  ends when the workflow is deleted.
- `workspaces.office_workflow_id` only says where new Office tasks are
  created.
- Office task identity and Kanban exclusion read the marker, together with
  the workspace's selected workflow (which selection always marks). "Is this
  an Office workspace" and "where do new Office tasks go" keep reading
  `office_workflow_id`.
- A workflow that holds tasks and is not an Office workflow cannot be selected,
  so selection never reclassifies existing Kanban tasks.

## Alternatives considered

- **Treat only the selected workflow as Office.** Simplest, but breaks tasks in
  flight on every switch.
- **Derive membership from `workflows.style = 'office'`.** Reuses a column, but
  turns a presentation hint into a behavioral invariant, against ADR-0004.
- **Derive membership from task history** (any workflow that ever held an
  Office task). Needs a history scan on every read and cannot classify an
  empty copy before it is selected.

## Consequences

- One schema column and an idempotent backfill reproduce today's membership
  exactly, so workspaces that never switch see no change.
- Office copies and imports stay off the Kanban board before they are
  selected, and previously selected workflows stay Office while their tasks
  finish.
- Every membership reader must use the marker. The predicate stays in one
  place (`IsFromOfficePredicate`) so new readers do not reintroduce
  `office_workflow_id` equality.
- A workflow wrongly marked Office cannot be unmarked through the product; the
  workaround is exporting it, removing `office: true`, and importing the file.

## Related

- [ADR-0004 Task model unification](0004-task-model-unification.md)
- [Office workflow selection requirements](../specs/office/requirements/office-workflow-selection.md)
- [Office workflow selection system design](../specs/office/system-design/office-workflow-selection.md)
