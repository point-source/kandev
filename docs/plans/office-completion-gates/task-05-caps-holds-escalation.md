---
id: "05-limits-holds-escalation"
title: "Limits, holds and escalation"
status: done
wave: 4
depends_on: ["04-result-routing"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-COMPLETION-GATES-003
  - REQ-OFFICE-COMPLETION-GATES-005
  - REQ-OFFICE-COMPLETION-GATES-007
acceptance_criteria:
  - AC-OFFICE-COMPLETION-GATES-003.2
  - AC-OFFICE-COMPLETION-GATES-005.3
  - AC-OFFICE-COMPLETION-GATES-007.1
  - AC-OFFICE-COMPLETION-GATES-007.2
  - AC-OFFICE-COMPLETION-GATES-007.3
  - AC-OFFICE-COMPLETION-GATES-007.4
  - AC-OFFICE-COMPLETION-GATES-007.5
  - AC-OFFICE-COMPLETION-GATES-007.6
system_design:
  - ../../specs/office/system-design/completion-gates.md
---

# Task 05: Limits, holds and escalation

## Summary

Resolve limits, hold a criterion that reaches one, stop the assignee's runs
while held, escalate once to the owner, and let the owner or a person extend
the limit.

## In scope

- `office_completion_gate_settings` table (limits defaults and ceilings; default
  criteria column added here, consumed in Task 06), repository, service, and
  `GET/PUT /api/v1/office/workspaces/:wsId/completion-gates/settings`;
  workspace deletion cleanup and e2e reset.
- `completion_gate_limits` workflow-step field: model, DTO, migration, portable
  export/import, workflow sync retention.
- `completiongates.EffectiveLimits` and its use in the Task 04 router: at the limit,
  the evaluation command holds instead of counting.
- Run guard: `SchedulerService.ProcessRunGuard` skips assignee runs of a task
  with a held criterion (`completion_gate_held`).
- Escalation: wakeup with idempotency key per hold occurrence, recipient
  resolution (owner agent, CEO, or Inbox), prompt
  `prompt_completion_gate_held.go` with result history.
- Inbox projections `completion_gate_protected_path` and
  `completion_gate_held` in `service_inbox.go`.
- Extension: runtime route plus CLI `kandev task gate extend`, owner or CEO
  only, assignee refused, within ceiling; human route above ceiling.
- Metric `office_completion_gate_hold_total{reason}`.

## Out of scope

- UI (Tasks 07 and 08).

## Acceptance

- With limits of 1, a second CI failure holds the criterion, the task does not
  move, assignee runs are skipped, and the owner agent receives exactly one
  wake even when the hold event is redelivered.
- A human-owned hold and every protected-path hold appear once in the Inbox and
  disappear when released.
- The assignee cannot extend; the owner can within the ceiling; a person can
  above it; each is in the gate history.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/office/... ./internal/workflow/... -run 'CompletionGate|Limits|Inbox|RunGuard|GateExtend')
(cd apps/backend && go test -trimpath ./cmd/agentctl/... -run 'Gate')
(cd apps/backend && go run ./cmd/sqlguard ./internal && go test -race -trimpath ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Files likely touched

- `apps/backend/internal/office/repository/sqlite/` (settings table, workspace deletion)
- New `apps/backend/internal/office/completiongates/{limits.go,escalation.go,settings.go}`
- `apps/backend/internal/office/scheduler/run_processing.go`
- `apps/backend/internal/office/service/` (wakeup, prompt)
- `apps/backend/internal/office/dashboard/service_inbox.go`, `handler.go`
- `apps/backend/internal/office/runtime/handler.go`, `actions.go`
- `apps/backend/cmd/agentctl/kandev_task.go`
- `apps/backend/internal/workflow/models/`, step repository and export/import
- `apps/backend/cmd/kandev/e2e_reset.go` or `internal/backendapp/e2e_reset.go`

## Dependencies

Task 04.

## Risks

- The run guard must not block escalation runs or reviewer runs.
- Step field round-trip through export/import and sync needs explicit tests.

## Results

Done. `office_completion_gate_settings` stores default criteria, limit defaults
and ceilings (workspace deletion and e2e reset clean it), served by
`GET/PUT /api/v1/office/workspaces/:wsId/completion-gates/settings`. Workflow
steps carry an optional `completion_gate_limits` (repository, clone, DTO,
step events, controller, export/import and sync). `LimitsPolicy` resolves
criterion, completing step, workspace default, then 3, within the ceiling
unless a person confirmed the value; the router uses it for attempt and rework
limits. `SchedulerService.ProcessRunGuard` skips the assignee's runs on a task
with a held criterion. Holds escalate once through an idempotent Office run
(`task_completion_gate_held`, registered and classified unattended) to the
owner agent or the workspace CEO, with a dedicated prompt; person-owned limit
holds and every protected-path hold appear as Inbox items. Limit extension is one
route, `POST /api/v1/office/tasks/:id/completion-gate/criteria/:criterionId/extend`:
an agent JWT goes through the owner/CEO check within the ceiling (CLI
`kandev task gate extend`), a person may go above it and is recorded as the
confirmer.

Verification: Office, workflow, agentctl and task DTO packages pass;
sqlguard and race store-conformance pass; changed-Go lint reports 0 issues.
