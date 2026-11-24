---
id: "04-definition-save"
title: "Whole-workflow Office definition save"
status: done
wave: 3
depends_on:
  - "01-office-workflow-marker"
  - "02-office-workflow-validation"
  - "03-selection-api"
plan: "plan.md"
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-002
  - REQ-OFFICE-WORKFLOW-SELECT-003
  - REQ-OFFICE-WORKFLOW-SELECT-006
acceptance_criteria:
  - AC-OFFICE-WORKFLOW-SELECT-002.2
  - AC-OFFICE-WORKFLOW-SELECT-003.3
  - AC-OFFICE-WORKFLOW-SELECT-003.4
  - AC-OFFICE-WORKFLOW-SELECT-006.1
  - AC-OFFICE-WORKFLOW-SELECT-006.2
  - AC-OFFICE-WORKFLOW-SELECT-006.3
  - AC-OFFICE-WORKFLOW-SELECT-006.4
  - AC-OFFICE-WORKFLOW-SELECT-006.5
system_design:
  - ../../specs/office/system-design/office-workflow-selection.md
---

# Task 04: Whole-workflow Office definition save

## Summary

Add `POST /api/v1/workspaces/:id/office-workflows` and
`PUT /api/v1/workflows/:id/definition`, which create or replace an Office copy
and all its steps in one transaction, validating the result when the target
is the selected Office workflow. Guard single-step writes to built-in and
selected Office workflows.

## In scope

- `internal/workflow/service/office_definition.go`: key-to-ID mapping,
  reference rewrite, `ValidateStepEventsWithRouting`, `expected_updated_at`
  conflict, `step_holds_tasks`, selected-workflow validation.
- Repository transaction: workspace then workflow lock, metadata update, step
  update/insert/delete with `ClearStepReferences`, two-phase positions.
- Created copies: `is_office = 1`, `is_system = 0`, `hidden = 0`,
  `style = 'office'`, manual source, no template ID.
- Guards in `CreateStep`, `UpdateStep`, `DeleteStep`, `ReorderSteps`,
  `CreateStepsFromTemplate` (`built_in_read_only`,
  `selected_requires_definition_save`); `SetSeatAuthorExclusion` unaffected.
- Workflow and step events published after commit.
- Extend the task 03 PostgreSQL concurrency test with the save side.

## Out of scope

- Web save integration (task 06).

## Acceptance

- An invalid save of the selected Office workflow returns 422 with problems
  and leaves workflow and steps byte-identical.
- A valid save applies all changes in one commit; a failure mid-write leaves
  nothing changed.
- Single-step writes to the selected or built-in Office workflow return 409;
  the routing author-exclusion endpoint still succeeds on them.

## Verification

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/workflow/service/... \
  ./internal/workflow/repository/... ./internal/workflow/handlers/... -run 'Office|Definition|Guard')
(cd apps/backend && go test -trimpath -count=1 ./internal/office/dashboard/... -run 'AuthorExclusion')
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test -trimpath -count=1 \
  ./internal/task/repository/sqlite/... ./internal/workflow/repository/... -run 'Serialize|Definition')
(cd apps/backend && go run ./cmd/sqlguard ./internal)
bash scripts/lint-go-changed <changed-go-files>
```

## Files likely touched

- `apps/backend/internal/workflow/service/office_definition.go` (new) and test
- `apps/backend/internal/workflow/service/office_step_guard_test.go` (new)
- `apps/backend/internal/workflow/service/service.go`
- `apps/backend/internal/workflow/repository/office_definition.go` (new) and test
- `apps/backend/internal/workflow/handlers/handlers.go`

## Dependencies

Tasks 01, 02, 03 (lock helper).

## Risks

- Step position uniqueness during rewrite; reuse `repository/reorder.go`.
- Session targets referencing steps must be remapped like `move_to_step`.

## Parallelism

`sequential`

## Inputs

- Design sections "Definition save", "Single-step write guard", "Locking".

## Results

- `go test ./internal/workflow/service/... ./internal/workflow/repository/... ./internal/workflow/handlers/...`: pass (definition save, atomic refusal, removed-step guard, single-step guard, routing control still allowed).
- `TestPostgresSelectionAndSaveSerialize` covers the save side: pass.
- `scripts/lint-go-changed`: 0 issues.
