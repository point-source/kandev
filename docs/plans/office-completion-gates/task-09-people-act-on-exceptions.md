---
id: "09-people-act-on-exceptions"
title: "People act on exceptions"
status: done
wave: 7
depends_on: ["03-review-at-head", "05-limits-holds-escalation", "07-gate-status-ui", "08-creation-ui"]
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-006
  - REQ-OFFICE-COMPLETION-GATES-001
  - REQ-OFFICE-COMPLETION-GATES-004
  - REQ-OFFICE-COMPLETION-GATES-005
  - REQ-OFFICE-COMPLETION-GATES-007
  - REQ-OFFICE-COMPLETION-GATES-008
  - REQ-OFFICE-COMPLETION-GATES-009
acceptance_criteria:
  - AC-TASKS-COMPLETION-006.6
  - AC-TASKS-COMPLETION-006.7
  - AC-TASKS-COMPLETION-006.10
  - AC-OFFICE-COMPLETION-GATES-001.1
  - AC-OFFICE-COMPLETION-GATES-001.3
  - AC-OFFICE-COMPLETION-GATES-004.2
  - AC-OFFICE-COMPLETION-GATES-004.6
  - AC-OFFICE-COMPLETION-GATES-005.5
  - AC-OFFICE-COMPLETION-GATES-007.1
  - AC-OFFICE-COMPLETION-GATES-007.7
  - AC-OFFICE-COMPLETION-GATES-008.4
  - AC-OFFICE-COMPLETION-GATES-008.8
  - AC-OFFICE-COMPLETION-GATES-008.9
  - AC-OFFICE-COMPLETION-GATES-008.10
  - AC-OFFICE-COMPLETION-GATES-009.2
  - AC-OFFICE-COMPLETION-GATES-009.3
  - AC-OFFICE-COMPLETION-GATES-009.4
system_design:
  - ../../specs/office/system-design/completion-gates.md
  - ../../specs/office/system-design/completion-gates-01.md
  - ../../specs/tasks/system-design/coordination-controls.md
---

# Task 09: People act on exceptions

## Summary

Rework the delivered gate to the rewritten #5351: review counts only required
reviewer seats; a person accepts a held review or sends a review back at the
commit shown; kinds become `check` (with runner `github`) and `review`;
verdict sources replace confidence levels; holds carry a `hold_kind` and show
**Needs you**; "limit" replaces "cap"; and a review no agent can approve holds
as `no_reviewer_available` or waits for a parked reviewer.

## In scope

- Renames across storage, API, CLI, MCP, UI, i18n, docs and tests: kinds and
  criterion IDs (`github_check`, `review`), limits (`verification_limit`,
  `rework_limit`, `review_wait_limit`, `extend_limits`,
  `completion_limit_above_ceiling`), `hold_kind` and its values.
- Deletions: the person review shortcut, confidence levels and the weakest
  level, and the unused held/failed/cannot-evaluate summary counts.
- Task controls `hold`, `release_hold` by kind, `send_back` (check or review)
  and `accept`, bound to the heads the person saw; snapshot `heads_revision`;
  verdict source in history.
- Person routes `allow`, `send-back`, `accept`; review Send back routes like a
  seat rejection; Accept retries completion without moving the task.
- Optional review wait limit (criterion, step, workspace; no ceiling).
- No reviewer available: seat-caster and scheduler observers, the reviewer
  wait record and the review wait sweep.
- Refusal of a review criterion without a review stage on edit and on the
  plugin write path.
- UI: hold detail lines, **Needs you**, per-hold actions shared by the task
  page and a single Inbox item type, review wait limit fields, the auth-off
  warning moved to where people act.

## Exclusions

- The Kandev check runner (phase 2; design recorded in system design part 1).

## Implementation acceptance

1. A held review is accepted or a review is sent back only at the heads shown,
   recorded as the person's verdict, and Send back routes like a seat
   rejection.
2. A review no agent can approve holds at once for permanent causes and waits,
   visibly, for a parked reviewer unless a review wait limit runs out.
3. Old names (kind values, cap fields, confidence levels, held reason) no
   longer appear on any surface.

## Verification

```bash
cd apps/backend && go test -trimpath ./internal/office/completiongates/ ./internal/office/scheduler/ ./internal/office/engine_adapters/ ./internal/office/service/ ./internal/task/models/ ./internal/task/service/ ./internal/task/handlers/ ./cmd/agentctl/ ./internal/backendapp/
cd apps/backend && go test -trimpath ./internal/task/repository/sqlite/ -run 'TestHost|TestHold|TestAccept|TestSendBack|Completion'
cd apps/web && pnpm exec vitest run lib/completion-gates lib/api/domains/task-completion-gates-actions.test.ts components/task/simple app/office
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm e2e:run --host --project chromium tests/office/completion-gates-review.spec.ts tests/office/completion-gates-ci.spec.ts tests/office/completion-gates-override.spec.ts tests/office/completion-gates-create.spec.ts
cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/office/mobile-completion-gates-review.spec.ts tests/office/mobile-completion-gates-ci.spec.ts tests/office/mobile-completion-gates-override.spec.ts tests/office/mobile-completion-gates-create.spec.ts
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Likely files

- `apps/backend/internal/office/completiongates/` (`reviewers.go`,
  `review_verifier.go`, `service.go`, `handler.go`, `limits.go`, `create.go`)
- `apps/backend/internal/task/models/completion_gates.go`,
  `internal/task/repository/sqlite/completion_gate_host.go`
- `apps/backend/internal/office/scheduler/dispatch_author_exclusion.go`,
  `internal/office/engine_adapters/seat_caster.go`
- `apps/web/lib/completion-gates/`, `components/task/simple/completion-*.tsx`,
  `app/office/inbox/inbox-completion-gate-actions.tsx`

## Risks

- Seat-cast timing: the hold is raised from the caster, the one synchronous
  point that knows the seat, so it never races the seat write.
- A reviewer added later is seated by a one-minute sweep, so the hold can take
  up to a minute to clear.

## Results

Implemented as scoped. The delivered E2E shows the caster hold on a real step
entry: a task assigned to the workspace's only agent holds its review as
`no_reviewer_available` (`assignee_only`) on entering Review, and a person
accepts it from the Inbox. One defect was found and fixed during E2E: a hold
raised before any review result had no heads, so Accept from the Inbox had
nothing to pin; holds now record the live heads.

Follow-up decided with the operator: a reviewer agent added or activated
after the hold now clears it. A one-minute sweep re-casts the reviewer role
for reviews held as `assignee_only` or `no_reviewer_role`, registers the new
reviewer (claiming the fallback seat), queues its review run and releases the
hold; the review verifier ignores the assignee's seat. Covered by unit tests
and an E2E that adds a specialist agent to a held task.
