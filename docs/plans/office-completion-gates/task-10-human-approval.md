---
id: "10-human-approval"
title: "Human approval criterion"
status: done
wave: 8
depends_on: ["09-people-act-on-exceptions"]
plan: "plan.md"
requirements:
  - REQ-TASKS-COMPLETION-006
  - REQ-OFFICE-COMPLETION-GATES-001
  - REQ-OFFICE-COMPLETION-GATES-010
acceptance_criteria:
  - AC-TASKS-COMPLETION-006.10
  - AC-OFFICE-COMPLETION-GATES-001.1
  - AC-OFFICE-COMPLETION-GATES-010.1
  - AC-OFFICE-COMPLETION-GATES-010.2
  - AC-OFFICE-COMPLETION-GATES-010.3
  - AC-OFFICE-COMPLETION-GATES-010.4
  - AC-OFFICE-COMPLETION-GATES-010.5
  - AC-OFFICE-COMPLETION-GATES-010.6
  - AC-OFFICE-COMPLETION-GATES-010.7
system_design:
  - ../../specs/office/system-design/completion-gates.md
  - ../../specs/office/system-design/completion-gates-01.md
  - ../../specs/tasks/system-design/coordination-controls.md
---

# Task 10: Human approval criterion

## Summary

Add a third criterion kind, `approval`, that passes only when a person
approves the current heads. A person is asked once the task has left the work
stage and every other criterion passes. Approve completes the task through its
normal transition; Send back routes like a review send back. The person's
"review requested" Inbox items give way to the approval item.

## In scope

- Kind `approval`: model constant, one-per-kind validation, config with
  `rework_limit` only, refusal of a runner, evidence still reserved.
- Creation: Office new-task dialog, workspace defaults and settings card,
  `kandev task create --require approval`, MCP `completion_criteria` kind
  `approval`, routine templates.
- `ApprovalVerifier`: records `pending` / `awaiting_criteria` (detail names
  what it waits for) or `pending` / `awaiting_person` at the live heads; runs
  from the router's existing subscriptions and after other criteria record.
- Control and route `approve` (optional note, heads-bound, refused unless
  `awaiting_person` at the heads shown); Send back on an approval through
  `routeSendBack`; stale on a new head via `OnHeadsChanged`.
- Inbox: `completion_gate_approval` items; no `task_review_request` items for
  the person.
- UI: **Human approval** row states and actions on the task page; Inbox
  **Approval needed** row; Approve dialog with an optional note. Desktop and
  phone; copy in seven locales.
- Worker prompt section names the criterion.

## Exclusions

- Named approvers, several approvers, or an approval quorum.
- Escalating or timing out an unanswered approval.

## ASCII UI preview

Excerpt of the plan's `UI-08` to `UI-10`; see [plan.md](plan.md#ascii-ui-preview).

```text
| Human approval         [Pending]                              |
|   Asked after CI check and Review pass                        |
|---------------------------------------------------------------|
| Human approval         [Needs you]                            |
|   Waiting for your approval · a1b2c3d                         |
|   [ Approve at this commit ]  [ Send back ]                   |
|---------------------------------------------------------------|
| Human approval         [Passed]                               |
|   Approved by you: Checked the redirect flow · a1b2c3d        |
```

## Implementation acceptance

1. A task with an approval criterion, alone or with a check and a review,
   cannot complete until a person approves its current heads, and is asked
   only after it left Work and the other criteria pass.
2. Approve at the shown heads completes the task through its normal
   transition; at moved heads it is refused with 409; Send back counts a round
   and wakes the worker with the reason.
3. The person's Inbox shows **Approval needed** for asked approvals and no
   "your review requested" items.

## Verification

```bash
cd apps/backend && go test -trimpath ./internal/office/completiongates/ ./internal/office/dashboard/ ./internal/office/service/ ./internal/task/models/ ./internal/task/service/ ./internal/task/handlers/ ./internal/mcp/... ./cmd/agentctl/ ./internal/backendapp/
cd apps/backend && go test -trimpath ./internal/task/repository/sqlite/ -run 'TestHost|TestAccept|TestApprove|TestSendBack|Completion'
cd apps/web && pnpm exec vitest run lib/completion-gates lib/api/domains components/task/simple app/office
cd apps/web && pnpm run i18n:check
cd apps/web && pnpm e2e:run --host --project chromium tests/office/completion-gates-approval.spec.ts tests/office/completion-gates-review.spec.ts tests/office/completion-gates-create.spec.ts
cd apps/web && pnpm e2e:run --host --project mobile-chrome tests/office/mobile-completion-gates-approval.spec.ts tests/office/mobile-completion-gates-create.spec.ts
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Likely files

- `apps/backend/internal/task/models/completion_gates.go`,
  `internal/task/repository/sqlite/completion_gate_host.go`,
  `internal/task/repository/sqlite/completion_gates.go`
- `apps/backend/internal/office/completiongates/` (new `approval_verifier.go`,
  `router.go`, `controls.go`, `handler.go`, `escalation.go`, `create.go`)
- `apps/backend/internal/office/dashboard/service_inbox.go`
- `apps/backend/cmd/agentctl/kandev_task_gate.go`,
  `internal/mcp/server/server.go`, `internal/office/service/prompt_completion_gate.go`
- `apps/web/lib/completion-gates/`, `components/task/simple/completion-*.tsx`,
  `app/office/inbox/`, `app/office/components/new-task-dialog.tsx`,
  workspace settings card, locales

## Risks

- Asking depends on placement and on the other criteria, so the verifier must
  re-run on every input that changes either; a missed trigger leaves the ask
  unshown. Tests cover each trigger.
- Removing the person's review-request items changes the Inbox for tasks
  without criteria too; they were never actionable by a person in Office.

## Results

Implemented as scoped.

- Backend: kind `approval` (rework limit only, one per task), control
  `approve` bound to the heads shown, `Router.EvaluateApproval` recording
  `awaiting_criteria` (detail `work_stage` or the blocking kinds) or
  `awaiting_person`, and the `approve` route. A workflow without a work stage
  counts as past it. A push needs no separate stale record: the next
  evaluation records at the new heads. `Service.Approve` re-evaluates the ask
  before applying, so an approval whose other criteria stopped passing is
  refused with 409.
- Inbox: `completion_gate_approval` items; the person's `task_review_request`
  items and count are gone; agent inboxes are unchanged.
- Creation: new-task dialog and workspace settings checkbox, `--require
  approval` (a verification limit is not sent for it), MCP kind `approval`.
- UI: **Human approval** row states, rework-only counts, Approve dialog or
  bottom sheet with an optional note, Inbox **Approval needed** row. Copy in
  seven locales.
- Public docs: `office-tasks.md` and `automation-and-mcp.md`.

Verification: the listed Go packages, web unit tests (201 files),
typecheck, Oxlint, `i18n:check`, `i18n:ratchet`; E2E chromium approval 3/3,
review and create 4/4; mobile-chrome approval and create 2/2.

Review follow-up (2026-11-22):

- Evaluation and the `approve` control hold a per-task lock, so a stale
  evaluation cannot record an ask after a newer one.
- An approval needs a workflow step after Work that does not complete the
  task (`completion_criteria_requires_approval_stage`).
- With no linked pull request the approval waits with detail `pull_request`;
  `github.task_pr.deleted` re-evaluates, so detaching the last pull request
  withdraws the ask and its Inbox item. The task page offers Send back only at
  a shown commit.
- Every Office `task.updated` re-evaluates the approval, covering criteria
  changes and attested evidence without a move.
- The held-review rebind applies only to the same hold at the heads read and
  only at the live heads (`RebindFromSubject`).
- Invalid criteria sets answer 400 `invalid_completion_gate` instead of 500,
  and extending a verification limit on an approval is refused.
- Not changed: the completion retry re-applies only a satisfied quorum-guarded
  transition. Replaying an unguarded `on_turn_complete` transition could finish
  a task while its step's turn still runs; the requirement and public docs now
  say the person moves such a task to Done.
