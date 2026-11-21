---
id: "02-ci-verifier"
title: "CI verifier with protected paths"
status: done
wave: 2
depends_on: ["01-host-verified-criteria"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-COMPLETION-GATES-002
  - REQ-OFFICE-COMPLETION-GATES-003
acceptance_criteria:
  - AC-OFFICE-COMPLETION-GATES-002.1
  - AC-OFFICE-COMPLETION-GATES-002.2
  - AC-OFFICE-COMPLETION-GATES-002.3
  - AC-OFFICE-COMPLETION-GATES-002.4
  - AC-OFFICE-COMPLETION-GATES-002.5
  - AC-OFFICE-COMPLETION-GATES-002.6
  - AC-OFFICE-COMPLETION-GATES-002.7
  - AC-OFFICE-COMPLETION-GATES-003.1
  - AC-OFFICE-COMPLETION-GATES-003.3
  - AC-OFFICE-COMPLETION-GATES-003.5
system_design:
  - ../../specs/office/system-design/completion-gates.md
---

# Task 02: CI verifier with protected paths

## Summary

Kandev verifies `ci_at_head` criteria from PR checks at each linked head,
using PR-status check selection, and holds the criterion when the PR touches a
protected path.

## In scope

- `github.Service.ObserveChecksAtHead` over the REST check-selection path,
  bypassing the feedback cache, returning the observation struct in the design.
- `github.Service.ChangedFilesAtHead` with head re-read; paginate
  `PATClient.ListPRFiles` with `getPaginated`; completeness against
  `changed_files` and the 3000 cap; mock client support.
- New package `internal/office/completiongates`: `CIVerifier` subscribed to
  `github.task_pr.updated`, per-task keyed serialization and coalescing,
  aggregation rules 1 to 6, protected-path matching with `doublestar`
  (filename and rename old path), result recording through
  `RecordTaskCompletionEvaluation`.
- Missing-check grace (10 minutes from `result_first_seen_at`), sweep on the
  Office scheduler's one-minute tick (100 tasks per tick), startup scan.
- `AllowTaskCompletionProtectedPaths` triggers re-evaluation; human route
  `POST /api/v1/office/tasks/:id/completion-gate/criteria/:criterionId/protected-paths`
  with `action: allow` (`send_back` is wired in Task 04).
- Wiring in `backendapp` with Start/Stop ownership and `goleak` in the new
  package's `TestMain`.
- Metrics `office_completion_gate_evaluation_total{kind,result}`.

## Out of scope

- Moving the task on `fail`, counters and holds beyond the protected hold
  (Tasks 04 and 05). Inbox items (Task 05).

## Acceptance

- With the mock GitHub client: success at head records `pass`; a failed check
  records `fail` naming it; running checks stay `pending`; no checks for 10
  minutes record `cannot_evaluate` / `checks_missing`; a new head re-evaluates
  and the old `pass` no longer satisfies the gate.
- A PR touching `tests/**` with that path protected holds the criterion with
  the matching files; Allow for those heads leads to `pass` when checks pass; a
  new head touching it holds again.
- An incomplete file list records `cannot_evaluate` /
  `changed_files_incomplete`, never `pass`.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/github/... ./internal/office/completiongates/... ./internal/backendapp/... -run 'CompletionGate|ChecksAtHead|ChangedFiles|ListPRFiles')
(cd apps/backend && go test -race -trimpath ./internal/office/completiongates/...)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Files likely touched

- `apps/backend/internal/github/client_helpers.go`, `pat_client.go`, `mock_client.go`, `mock_controller.go`, new `service_checks_at_head.go`
- New `apps/backend/internal/office/completiongates/{ci_verifier.go,aggregate.go,protected_paths.go,sweep.go,metrics.go}`
- `apps/backend/internal/office/dashboard/handler.go` (protected-paths route)
- `apps/backend/internal/backendapp/` wiring

## Dependencies

Task 01.

## Risks

- REST reads per PR update can hit rate limits; coalesce per task and skip
  tasks without a CI criterion before any GitHub call.
- The mock path has no GraphQL, so production GraphQL rollups are not
  exercised; the verifier must not read `checks_state` from the row.

## Results

Done. `github.Service.ObserveChecksAtHead` and `ChangedFilesAtHead` use the
workspace automation client and the REST check-selection path;
`PATClient.ListPRFiles` now follows pagination, and the mock reports its
changed-file count when files are seeded. The new
`internal/office/completiongates` package holds the pure CI aggregation, a
per-task coalescing scheduler, the `github.task_pr.updated` subscriber, the
startup scan and one-minute missing-check sweep, and the human
protected-paths route (`action: allow`; `send_back` lands with Task 04).
`backendapp` adapts the GitHub service to `PRSource` and starts/stops the
service after readiness. Evaluations also carry `ReleaseHold`, so a clean new
head clears a protected-path hold. Task-repository `ListTaskIDsWithUnmetCompletionCriteria`
feeds the sweep.

Verification: github package, completiongates (with `-race` and goleak),
Office packages, backendapp scope/route tests and changed-Go lint pass. The
rework metric is wired in Task 04.
