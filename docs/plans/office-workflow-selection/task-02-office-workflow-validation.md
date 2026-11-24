---
id: "02-office-workflow-validation"
title: "Office workflow validation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-005
acceptance_criteria:
  - AC-OFFICE-WORKFLOW-SELECT-005.1
  - AC-OFFICE-WORKFLOW-SELECT-005.2
  - AC-OFFICE-WORKFLOW-SELECT-005.3
  - AC-OFFICE-WORKFLOW-SELECT-005.4
  - AC-OFFICE-WORKFLOW-SELECT-005.5
  - AC-OFFICE-WORKFLOW-SELECT-005.7
system_design:
  - ../../specs/office/system-design/office-workflow-selection.md
---

# Task 02: Office workflow validation

## Summary

Add the pure `ValidateOfficeWorkflow` function and `OfficeWorkflowProblem`
type that every Office write path uses. It returns all failing rules with
stable codes and the step, role, action or count each one names.

## In scope

- `internal/workflow/models/office_validation.go` with the six rule codes in
  the design's Validation table.
- Reachability over `move_to_step`, `move_to_next`, `move_to_previous` in all
  step events.
- Deterministic ordering by step position, then code.
- Table tests per rule and a test that the embedded `office-default` template
  passes.

## Out of scope

- Localized copy (task 06) and HTTP mapping (tasks 03 and 04).

## Acceptance

- Each rule has a passing and a failing table case.
- A workflow failing several rules returns every problem in order.
- The embedded `office-default` template converted to steps returns no
  problems.

## Verification

```bash
(cd apps/backend && go test -trimpath -count=1 ./internal/workflow/models/... -run 'Office')
bash scripts/lint-go-changed apps/backend/internal/workflow/models/office_validation.go apps/backend/internal/workflow/models/office_validation_test.go
```

## Files likely touched

- `apps/backend/internal/workflow/models/office_validation.go` (new)
- `apps/backend/internal/workflow/models/office_validation_test.go` (new)

## Dependencies

None.

## Risks

- `move_to_next` / `move_to_previous` reachability depends on position order;
  compute from sorted positions, not slice order.

## Parallelism

`parallel-safe` with task 01.

## Inputs

- Design section "Validation".
- `config/workflows/office-default.yml`, `models.ReviewAgentProfileConfigKey`.

## Results

- `go test ./internal/workflow/models/... ./config/workflows/...`: pass, including `TestOfficeDefaultTemplatePassesOfficeValidation`.
- `scripts/lint-go-changed`: 0 issues.
- Review fix: `move_to_next` / `move_to_previous` reachability follows the runtime's strict position rule (`TestValidateOfficeWorkflowMoveToNextRequiresHigherPosition`).
