---
id: "01-host-verified-criteria"
title: "Host-verified criteria in the task gate"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-006
  - REQ-OFFICE-COMPLETION-GATES-005
acceptance_criteria:
  - AC-TASKS-COMPLETION-006.1
  - AC-TASKS-COMPLETION-006.2
  - AC-TASKS-COMPLETION-006.3
  - AC-TASKS-COMPLETION-006.4
  - AC-TASKS-COMPLETION-006.5
  - AC-TASKS-COMPLETION-006.6
  - AC-TASKS-COMPLETION-006.7
  - AC-TASKS-COMPLETION-006.8
  - AC-TASKS-COMPLETION-006.9
  - AC-OFFICE-COMPLETION-GATES-005.1
system_design:
  - ../../specs/tasks/system-design/coordination-controls.md
  - ../../specs/office/system-design/completion-gates.md
---

# Task 01: Host-verified criteria in the task gate

## Summary

Extend the task-owned completion gate so a criterion has a kind, host-verified
kinds accept results only from in-process commands, proof is pinned to the set
of linked PR heads, and each criterion carries a result, owner, limits, counters
and an optional hold. Criteria can be written in the task's create
transaction.

## In scope

- Models (`task/models/completion_gates.go`): kind constants, `github_pr_heads`
  subject, result and confidence constants, `CompletionCriterionConfig`,
  result/owner/counter/hold fields on `TaskCompletionCriterion`,
  `weakest_confidence` and `held_count` on the snapshot and summary,
  `CompletionGateCaller` with `WithCompletionGateCaller`.
- Schema: `ADD COLUMN` migrations listed in the design's Schema table, in
  `base_migrations.go`, with fresh-DB and replay tests on SQLite and an
  env-gated PostgreSQL test.
- Repository: canonical PR-heads revision builder shared by Go and the batch
  reader; live check in `evidenceSubjectCurrentTx` and
  `completion_gate_batch.go`; blockers `pr_missing`, `pr_merged_unverified`,
  `held`; equivalence includes kind and protected paths; revision change
  resets counters, hold, protected approval and result.
- Commands: `RecordTaskCompletionEvaluation` (conditional on criterion and
  subject revisions, idempotent, counter increments, hold set/clear),
  `ReleaseTaskCompletionHold`, `ExtendTaskCompletionCaps`,
  `AllowTaskCompletionProtectedPaths`. History entries for each.
- Reservation: HTTP, Exact, and plugin Host verify paths refuse host kinds
  with `task_completion_verifier_reserved` (handler maps to 409).
- Self-verification: `SetTaskCompletionCriteria` refuses an assignee caller
  with `task_completion_self_verification`.
- Atomic creation: `CreateTaskRequest.CompletionCriteria` and one
  `insertCompletionCriteriaTx` used by every task-creating repository method.
- Web API types in `apps/web/lib/api/domains/task-completion-gates-api.ts`.

## Out of scope

- Verifiers, routing, creation surfaces, UI (Tasks 02 to 08).
- Office-specific validation (repository, review stage); Task 06.

## Acceptance

- A host-verified criterion cannot be verified through HTTP, plugin Host, or
  Exact paths with authentication off or on; an `attested` criterion behaves
  as before, proven by the existing gate tests passing unchanged.
- `RecordTaskCompletionEvaluation` passes only while the live PR heads equal
  the observed revision; a push, link or detach makes the guard report
  `evidence_stale`, and no linked PR reports `pr_missing`.
- A task created with criteria has them in the same commit; a refused criterion
  set leaves no task row.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/task/models/... ./internal/task/repository/... ./internal/task/service/... ./internal/task/handlers/... ./internal/plugins/...)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test -trimpath -run 'CompletionGate|CompletionCriteria|HostVerified' ./internal/task/repository/sqlite/...)
(cd apps/backend && go run ./cmd/sqlguard ./internal && go test -race -trimpath ./internal/persistence/storeconformance -count=1)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
(cd apps/web && pnpm run typecheck)
```

## Files likely touched

- `apps/backend/internal/task/models/completion_gates.go`
- `apps/backend/internal/task/repository/sqlite/completion_gates.go`, `completion_gate_batch.go`, `base_migrations.go`, task create methods in `task.go`, new `completion_gate_evaluation.go`
- `apps/backend/internal/task/repository/repoerrors/` (new sentinels)
- `apps/backend/internal/task/service/completion_gates.go`, `service_requests.go`, `service_tasks.go`
- `apps/backend/internal/task/handlers/completion_gate_handlers.go`
- `apps/backend/internal/plugins/host_completion_gates.go`
- `apps/web/lib/api/domains/task-completion-gates-api.ts`

## Dependencies

None.

## Risks

- Several repository methods create tasks (admission, child, transfer). Find
  them all with a test that creates through each and asserts criteria rows.
- Batch summary SQL must compute the same heads revision as Go; test both on
  multi-PR and detached rows.

## Results

Done. Criteria carry kind, config, owner, result, counters, hold and
protected approval (16 replayable `ADD COLUMN` migrations). Host kinds use the
`github_pr_heads` subject recomputed from linked, non-detached PR rows;
blockers add `pr_missing`, `pr_merged_unverified` and `held`. The existing
verify path (HTTP, Exact, plugin Host) refuses host kinds in the repository,
so every caller shares one check. `RecordTaskCompletionEvaluation` and
`ApplyTaskCompletionControl` are task-service methods with no route. Criteria
supplied on `CreateTaskRequest` are inserted by `insertTaskTx`, the single task
insert, so every create path is covered. Status summaries carry `held_count`
and `weakest_confidence`.

Verification: task models, repository, service, handlers, statussummary and
plugins packages pass; sqlguard and race store-conformance pass; changed-Go
lint reports 0 issues; web typecheck passes. The PostgreSQL run was not
possible: `KANDEV_TEST_POSTGRES_DSN` is unset in this environment.
