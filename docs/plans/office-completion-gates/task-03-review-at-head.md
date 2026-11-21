---
id: "03-review-at-head"
title: "Review pinned to the head"
status: done
wave: 2
depends_on: ["01-host-verified-criteria"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-COMPLETION-GATES-004
  - REQ-OFFICE-COMPLETION-GATES-005
acceptance_criteria:
  - AC-OFFICE-COMPLETION-GATES-004.1
  - AC-OFFICE-COMPLETION-GATES-004.2
  - AC-OFFICE-COMPLETION-GATES-004.4
  - AC-OFFICE-COMPLETION-GATES-005.2
  - AC-OFFICE-COMPLETION-GATES-005.4
system_design:
  - ../../specs/office/system-design/completion-gates.md
---

# Task 03: Review pinned to the head

## Summary

Reviewer and human approvals on a gated task store the PR heads they saw. The
review-at-head criterion passes when every decision-required reviewer seat has
approved the current heads. Assignee approvals never count.

## In scope

- `workflow_step_decisions.pr_heads` column (migration, replay tests,
  PostgreSQL), threaded through `RecordDecisionInput`, `engine.DecisionInfo`,
  `RecordStepDecision`, and decision reads.
- Heads capture in `RecordAgentDecision` and `recordTaskDecision` for tasks
  with a `review_at_head` criterion, using the shared heads reader from Task 01.
- `completiongates.ReviewVerifier.OnDecision` after commit: seat coverage at
  current heads → `pass` with verifier kind `review_seats`.
- Assignee fence: skip assignee decisions; runtime decision response notice
  `review_self_approval_ignored`; metric
  `office_completion_gate_verifier_refused_total{reason}`.

## Out of scope

- Returning a task to review when heads move, and rejection counting
  (Task 04).

## Acceptance

- Two reviewer seats approving at head A pass the criterion; a push to B makes
  it stale at the guard.
- A human approval (synthetic admin with auth off) counts as a seat approval
  at its stored heads.
- An approval from the assignee agent leaves the criterion unchanged and
  returns the notice.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/workflow/... ./internal/office/engine_dispatcher/... ./internal/office/dashboard/... ./internal/office/runtime/... ./internal/office/completiongates/...)
(cd apps/backend && KANDEV_TEST_POSTGRES_DSN="$KANDEV_TEST_POSTGRES_DSN" go test -trimpath -run 'StepDecision' ./internal/workflow/repository/...)
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Files likely touched

- `apps/backend/internal/workflow/models/phase2.go`, `workflow/repository/phase2_sqlite.go`
- `apps/backend/internal/workflow/engine/adapters.go`, `quorum.go`
- `apps/backend/internal/office/engine_dispatcher/dispatcher.go`
- `apps/backend/internal/office/dashboard/agent_decisions.go`, `decisions.go`
- `apps/backend/internal/office/runtime/decision.go`, `handler.go`
- New `apps/backend/internal/office/completiongates/review_verifier.go`

## Dependencies

Task 01. Parallel-safe with Task 02 (different files except the new package;
coordinate on `completiongates` package scaffolding).

## Risks

- Decision supersede on rework must not lose `pr_heads` for audit; superseded
  rows are simply excluded from coverage.

## Results

Done. `workflow_step_decisions.pr_heads` (replayable migration) flows through
`RecordDecisionInput`, `engine.DecisionInfo`, the decision adapter and both
decision reads. The dashboard gained a `ReviewHeadsHook`; both the agent and
the human decision paths store heads before the write and notify after it.
`completiongates.ReviewVerifier` implements the hook: assignee decisions get no
heads and return `review_self_approval_ignored` (also on the runtime response
as `notice`); an approval records `pass` when every decision-required reviewer
seat has an unsuperseded approval at the current heads, or when a person
approved the current heads; a rejection records `fail` / `review_rejected`.

Verification: workflow, engine dispatcher, dashboard, runtime, Office and
completiongates packages pass; changed-Go lint is clean apart from the rework
metric that Task 04 wires. The PostgreSQL decision test was not run because
`KANDEV_TEST_POSTGRES_DSN` is unset.
