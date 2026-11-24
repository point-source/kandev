---
id: "05-export-import"
title: "Office export and import"
status: done
wave: 2
depends_on:
  - "01-office-workflow-marker"
plan: "plan.md"
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-007
acceptance_criteria:
  - AC-OFFICE-WORKFLOW-SELECT-007.1
  - AC-OFFICE-WORKFLOW-SELECT-007.2
  - AC-OFFICE-WORKFLOW-SELECT-007.3
  - AC-OFFICE-WORKFLOW-SELECT-007.4
system_design:
  - ../../specs/office/system-design/office-workflow-selection.md
---

# Task 05: Office export and import

## Summary

Carry the Office marker through the portable workflow format. Export sets
`office: true`; import creates Office copies in Office workspaces and refuses
Office files into workspaces without Office before writing.

## In scope

- `models.WorkflowPortable.Office` (`yaml:"office,omitempty"`), export mapping.
- Import and import preview: `requires_office_workspace` refusal; create with
  `is_office = 1`, `style = 'office'`.
- Round-trip test: export a modified Office copy, import, compare steps,
  stage types, events and profiles.
- `docs/workflow-import-export.md`: remove the stale `stage_type` note,
  document `office`.

## Out of scope

- Web export/import UI changes beyond what task 06 needs.
- Workflow sync of Office workflows.

## Acceptance

- Exported YAML for an Office workflow contains `office: true` and no
  participants, decisions or runs.
- Importing into an Office workspace yields an `is_office` workflow; importing
  into a non-Office workspace writes nothing and returns the code.

## Verification

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/workflow/models/... \
  ./internal/workflow/service/... ./internal/workflow/handlers/... -run 'Export|Import')
bash scripts/lint-go-changed <changed-go-files>
```

## Files likely touched

- `apps/backend/internal/workflow/models/export.go`
- `apps/backend/internal/workflow/models/export_office_test.go` (new)
- `apps/backend/internal/workflow/service/service.go` (import/export)
- `apps/backend/internal/workflow/service/import_office_test.go` (new)
- `docs/workflow-import-export.md`

## Dependencies

Task 01.

## Risks

- Import creation path must set `is_office` in the same insert, not a
  follow-up update, so a failed import leaves no Kanban-visible workflow.

## Parallelism

`parallel-safe` with task 03 (disjoint files); not with task 04
(`workflow/service/service.go`).

## Inputs

- Design section "Export and import".

## Results

- `go test ./internal/workflow/models/... ./internal/workflow/service/... ./internal/workflow/handlers/...`: pass (`TestExportMarksOfficeWorkflows`, `TestImportOfficeWorkflowCreatesSelectableOfficeCopy`, `TestImportOfficeWorkflowIntoKanbanWorkspaceIsRefused`, HTTP 422 import refusal).
- `docs/workflow-import-export.md` updated.
