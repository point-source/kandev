---
id: "01-office-workflow-marker"
title: "Office workflow marker"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-001
acceptance_criteria:
  - AC-OFFICE-WORKFLOW-SELECT-001.1
  - AC-OFFICE-WORKFLOW-SELECT-001.2
  - AC-OFFICE-WORKFLOW-SELECT-001.3
system_design:
  - ../../specs/office/system-design/office-workflow-selection.md
---

# Task 01: Office workflow marker

## Summary

Add the backend-authoritative `workflows.is_office` marker, backfill it to
today's membership, and switch every Office membership reader from
`office_workflow_id` equality to the marker. No user-visible change for
existing workspaces.

## In scope

- Migration adding `workflows.is_office` and the idempotent backfill (design
  "Office marker", "Migration").
- `models.Workflow.IsOffice`, workflow scans and inserts, `dto.WorkflowDTO.IsOffice`,
  web `Workflow.is_office` type field.
- `office: true` in `config/workflows/office-default.yml`;
  `WorkflowTemplate.Office`; `createWorkflowFromTemplate` writes it.
- Readers: `isFromOfficeProjection`, `ListOfficeWorkflowIDs` replacing
  `GetOfficeWorkflowIDs` (callers `listWorkflows`, `boot_state.go`),
  `validateAssigneeTaskIsOffice`, `office/runtime/handoff.go`.
- Persistence upgrade fixture/manifest and store conformance updates.

## Out of scope

- Any API that sets the marker or changes selection (tasks 03 to 05).

## Acceptance

- After the backfill, exactly the workflows referenced by
  `office_workflow_id` and system `office-default` rows are marked.
- A task on a marked workflow that is no longer the workspace's
  `office_workflow_id` still reports `IsFromOffice` and is still excluded from
  Kanban listings and the boot payload.
- `rg "OfficeWorkflowID ==|== .*OfficeWorkflowID|office_workflow_id = "` finds
  no remaining membership comparison.

## Verification

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/task/repository/sqlite/... \
  ./internal/task/service/... ./internal/task/handlers/... ./internal/office/runtime/... \
  ./internal/backendapp/... ./internal/workflow/...)
(cd apps/backend && go run ./cmd/sqlguard ./internal)
(cd apps/backend && go test -trimpath -race ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed <changed-go-files>
```

## Files likely touched

- `apps/backend/internal/task/repository/sqlite/base_migrations.go`
- `apps/backend/internal/task/repository/sqlite/workflow.go`
- `apps/backend/internal/task/repository/sqlite/task.go`
- `apps/backend/internal/task/repository/sqlite/defaults.go`
- `apps/backend/internal/task/repository/sqlite/office_workflow_marker_test.go` (new)
- `apps/backend/internal/task/models/models.go`, `internal/task/dto/dto.go`
- `apps/backend/internal/task/service/service_resources.go`, `service_tasks.go`
- `apps/backend/internal/task/handlers/workflow_handlers.go`
- `apps/backend/internal/backendapp/boot_state.go`
- `apps/backend/internal/office/runtime/handoff.go`
- `apps/backend/internal/workflow/models/models.go`, `apps/backend/config/workflows/office-default.yml`, template loader
- `apps/web/lib/types/http.ts`

## Dependencies

None.

## Risks

- PostgreSQL migration path and upgrade fixture must match SQLite.
- `isFromOfficeProjection` runs in hot list queries; keep the `EXISTS` indexed
  by primary key.

## Parallelism

`parallel-safe` with task 02 (disjoint files).

## Inputs

- Design sections "Office marker", "Migration", "Membership readers".
- ADR `docs/decisions/2026-11-24-office-workflow-marker.md`.

## Results

- `go test ./internal/task/repository/sqlite/... ./internal/task/service/... ./internal/task/handlers/... ./internal/office/runtime/... ./internal/backendapp/... ./internal/workflow/...`: pass.
- Broad regression (`orchestrator`, `office/...`, `mcp/...`, `runs`, `automation`, `coordinator`): pass after adding `is_office` to hand-rolled test schemas.
- PostgreSQL (`KANDEV_TEST_POSTGRES_DSN`, local throwaway instance): `task/repository/sqlite` (45m timeout), `workflow/...`, `persistence/storeconformance`: pass.
- `scripts/lint-go-changed`: 0 issues.
- Deviation: membership readers use the marker **or** the workspace's selected workflow (equal in production, since selection marks); recorded in the design and ADR.
