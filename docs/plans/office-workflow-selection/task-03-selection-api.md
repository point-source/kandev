---
id: "03-selection-api"
title: "Office workflow selection API"
status: done
wave: 2
depends_on:
  - "01-office-workflow-marker"
  - "02-office-workflow-validation"
plan: "plan.md"
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-002
  - REQ-OFFICE-WORKFLOW-SELECT-004
  - REQ-OFFICE-WORKFLOW-SELECT-006
acceptance_criteria:
  - AC-OFFICE-WORKFLOW-SELECT-002.5
  - AC-OFFICE-WORKFLOW-SELECT-004.1
  - AC-OFFICE-WORKFLOW-SELECT-004.2
  - AC-OFFICE-WORKFLOW-SELECT-004.3
  - AC-OFFICE-WORKFLOW-SELECT-004.4
  - AC-OFFICE-WORKFLOW-SELECT-004.5
  - AC-OFFICE-WORKFLOW-SELECT-006.5
system_design:
  - ../../specs/office/system-design/office-workflow-selection.md
---

# Task 03: Office workflow selection API

## Summary

Add `GET` and `PUT /api/v1/workspaces/:id/office-workflow`. Selection runs in
one transaction that locks the workspace then the workflow, checks
eligibility, validates, and writes `office_workflow_id` and `is_office`. Guard
deletion of the built-in and selected Office workflows.

## In scope

- `internal/task/service/service_office_workflow.go`: candidates, select,
  `authz.ScopeWorkspaceManage`, `requires_office_workspace`.
- Repository `SelectOfficeWorkflow` transaction with the design's lock order.
- Handlers and 422 problem response; `events.WorkspaceUpdated` and
  `events.WorkflowUpdated` after commit.
- `DeleteWorkflow` refusals `built_in_read_only` and `selected_delete`.
- PostgreSQL concurrency test for selection versus save (uses the task 04
  lock helper if present; otherwise asserts selection-side locking only and
  task 04 extends it).

## Out of scope

- Definition save (task 04); web picker (task 06).

## Acceptance

- Selecting a valid candidate changes the workspace and makes the next Office
  task created through `resolveOfficeWorkflow` start on that workflow.
- Ineligible or invalid choices return 422 with codes and leave both columns
  unchanged; a user without workspace-manage scope gets 403.
- Deleting the built-in or selected Office workflow is refused.

## Verification

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/task/service/... \
  ./internal/task/handlers/... ./internal/task/repository/sqlite/... -run 'Office')
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test -trimpath -count=1 \
  ./internal/task/repository/sqlite/... -run 'TestSelectionAndSaveSerialize')
(cd apps/backend && go run ./cmd/sqlguard ./internal)
bash scripts/lint-go-changed <changed-go-files>
```

## Files likely touched

- `apps/backend/internal/task/service/service_office_workflow.go` (new) and test
- `apps/backend/internal/task/repository/sqlite/office_workflow_selection.go` (new) and tests
- `apps/backend/internal/task/handlers/workspace_handlers.go`
- `apps/backend/internal/task/service/service_resources.go` (`DeleteWorkflow`)

## Dependencies

Tasks 01 and 02.

## Risks

- Lock order must match task 04 exactly to avoid deadlocks on PostgreSQL.

## Parallelism

`sequential` (shares the lock helper with task 04).

## Inputs

- Design sections "Selection API", "Locking", "Deletion guard".

## Results

- `go test ./internal/workflow/service/... ./internal/workflow/handlers/... ./internal/task/service/... ./internal/task/handlers/...`: pass.
- `TestPostgresSelectionAndSaveSerialize` on PostgreSQL: pass.
- Deviation: selection lives in the workflow service/handlers/repository (owner of step storage and locks); the task domain supplies `AuthorizeWorkspaceManage` and `PublishOfficeWorkflowChanged`. Recorded in the design.
