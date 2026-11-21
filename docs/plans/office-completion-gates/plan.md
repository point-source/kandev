---
created: 2026-11-20
status: in_progress
requirements:
  - REQ-TASKS-COMPLETION-006
  - REQ-OFFICE-COMPLETION-GATES-001
  - REQ-OFFICE-COMPLETION-GATES-002
  - REQ-OFFICE-COMPLETION-GATES-003
  - REQ-OFFICE-COMPLETION-GATES-004
  - REQ-OFFICE-COMPLETION-GATES-005
  - REQ-OFFICE-COMPLETION-GATES-006
  - REQ-OFFICE-COMPLETION-GATES-007
  - REQ-OFFICE-COMPLETION-GATES-008
system_design:
  - ../../specs/office/system-design/completion-gates.md
  - ../../specs/tasks/system-design/coordination-controls.md
legacy_specs: []
---

# Implementation Plan: Office completion gates

## Overview

Phase 1 of GitHub issue #5351. Office tasks gain two criteria that Kandev
verifies itself: CI at the PR head (with protected paths) and review at the
PR head. Failures route the work back, caps bound retries, holds escalate
once, and people see and recover gate state on the Office task page.

Settled with the user on 2026-11-20:

- The package covers all of phase 1. Phases 2 and 3 stay in the issue.
- Gate status and recovery appear on the Office task page only.
- A CI failure moves the task to the work stage; a stale review returns it to
  the review stage.
- Any human approval counts for review at head, including the synthetic admin
  with authentication off. The UI warns about this when a review criterion is
  configured with authentication off; the user decides whether to enable it.
- Assumption: "CI passes" uses PR-status check selection; no required-check
  list.

Order: 01 is the task-owned primitive every other order writes through. 02 and
03 add the two verifiers and can run in parallel. 04 adds routing and needs
both. 05 adds caps, holds and escalation on top of 04. 06 adds the creation
surfaces and workspace defaults and needs 05's settings table. 07 and 08 are
the UI.

Waves: 1 = 01; 2 = 02 and 03; 3 = 04; 4 = 05; 5 = 06 and 07; 6 = 08. Each
wave ends with a working product. After wave 2, criteria set through the
existing HTTP API are verified and enforced. After wave 3 the work routes
itself; after wave 4 it is bounded. After wave 5 every surface can create
gated tasks and people can recover them in the UI. Wave 6 adds the creation
and settings UI.

## Scope

### In scope

- `REQ-TASKS-COMPLETION-006`: criterion kinds, host-verified reservation, PR
  heads subject, results, counters, holds, owner, confidence, self-verification
  caller check, atomic creation.
- Office CI verifier (check selection at head, missing-check grace, protected
  paths, complete file lists), review verifier (heads on decisions), routing,
  caps, holds, escalation, cap extension, prompt section.
- Creation through the Office dialog, CLI, MCP and routine templates;
  workspace defaults, caps and ceilings; a step-level cap default.
- Office task page section, Inbox items, new-task dialog group, workspace
  settings card. Desktop and phone.

### Out of scope

- Acceptance scripts, disputes, quorum across providers, blind review,
  structured verdicts, confidence on dependents, reversibility policy, spot
  checks (#5351 phases 2 and 3).
- Gate controls on the generic task page.
- Branch-protection required checks.
- Pull-request webhooks. Verification follows the existing PR poller.

## Technical approach

See the [Office design](../../specs/office/system-design/completion-gates.md)
and the [coordination controls amendment](../../specs/tasks/system-design/coordination-controls.md#host-verified-criteria).
The [ADR](../../decisions/2026-11-20-host-verified-completion-criteria.md)
records why verification is in process and fenced by kind.

| Order | Main packages |
| --- | --- |
| 01 | `task/models`, `task/repository/sqlite`, `task/service`, `task/handlers`, `plugins` |
| 02 | `github`, new `office/completiongates`, `backendapp` |
| 03 | `workflow/repository`, `workflow/engine`, `office/engine_dispatcher`, `office/dashboard`, `office/completiongates` |
| 04 | `office/completiongates`, `office/service/prompt_builder.go` |
| 05 | `office/completiongates`, `office/scheduler`, `office/service`, `office/dashboard`, `office/runtime`, `office/repository/sqlite`, `workflow/models`, `cmd/agentctl` |
| 06 | `task/handlers`, `office/runtime`, `office/routines`, `mcp`, `backendapp`, `cmd/agentctl` |
| 07 | `apps/web/components/task/simple`, `apps/web/app/office/inbox`, `apps/web/lib/api` |
| 08 | `apps/web/app/office/components`, `apps/web/app/office/workspace/settings` |

## ASCII UI preview

Structure is required; wording is illustrative and localized in seven
languages. Spacing is not a pixel specification.

`UI-01: Completion criteria section`. Entry point: Office task page of a
gated task, main column, after the task action row. State: CI failed, review
stale.

```text
Desktop (main column; properties sidebar unchanged on the right)
+---------------------------------------------------------------+
| Completion criteria                                           |
|---------------------------------------------------------------|
| CI                     [x Failed]                             |
|   Failing: backend-tests, lint · a1b2c3d                      |
|   Rework rounds 1 of 3, verification attempts 0 of 3          |
|---------------------------------------------------------------|
| Review                 [! Stale]                              |
|   Needs approval · a1b2c3d                                    |
|   Rework rounds 0 of 3, verification attempts 0 of 3          |
|---------------------------------------------------------------|
|                                    [ Override completion ]    |
+---------------------------------------------------------------+
```

`UI-02: Held criterion actions`. Same section, protected-path hold and cap
hold.

```text
| CI                     [|| Held]                              |
|   Protected file changed: tests/webhook_test.go · a1b2c3d     |
|   [ Allow for this commit ]  [ Send back ]                    |
|---------------------------------------------------------------|
| Review                 [|| Held]                              |
|   Rework round limit reached · a1b2c3d                        |
|   [ Allow more attempts ]                                     |
```

`UI-03: Phone`. One scrolling column; the section is a full-width card.
Actions stack full width with 44-pixel targets. Dialogs open as bottom sheets.

```text
+-----------------------------+
| < KAN-42  Fix webhook dup   |  fixed header
|-----------------------------|
| ...title, description...    |  scrolls
| Completion criteria         |
| CI              [|| Held]   |
| Protected file changed:     |
| tests/webhook_test.go       |
| · a1b2c3d                   |
| [ Allow for this commit   ] |
| [ Send back               ] |
| Review          [Passed]    |
| Approved by [QA] QA agent   |
| · a1b2c3d                   |
| [ Override completion     ] |
+-----------------------------+
```

`UI-04: Done state`. Completed gated task.

```text
| Completion criteria                                     Done  |
| Completion criteria              Done by override · Sam       |
```

`UI-05: New-task dialog group`. Office new-task dialog, below the description.
Prefilled from workspace defaults; collapsed to a summary line until opened.

```text
| Completion criteria  (CI at head, Review at head)        [v]  |
|   [x] CI passes at PR head                                    |
|       Protected paths  [ tests/** ] [ .github/** ] [+ Add]    |
|   [x] Review at PR head                                       |
|   (!) Authentication is off. Any process on this machine can  |
|       approve this task as you, so this review can pass       |
|       without a real review. Turn on Authentication in        |
|       Settings > System > Feature Toggles to prevent this.    |
|   Max rework rounds [ 3 ]   Max verification attempts [ 3 ]   |
```

The warning appears only while **Review at PR head** is selected and
authentication is not enabled. It never disables **Create**.

`UI-06: Workspace settings card`. Office workspace settings page.

```text
| Completion gates                                              |
|   Default criteria for new tasks                              |
|   [x] CI at PR head   Protected paths [ tests/** ] [+ Add]    |
|   [ ] Review at PR head                                       |
|       (same authentication warning when checked and auth off) |
|   Default caps:  rework [ 3 ]  attempts [ 3 ]                 |
|   Ceilings:      rework [ 10 ] attempts [ 10 ]                |
|                                               [ Save ]        |
```

`UI-07: Inbox items`. Office Inbox, shared desktop and phone list.

```text
| (||) Protected test file changed · KAN-42 Fix webhook dup     |
|      tests/webhook_test.go          [ Allow ] [ Send back ]   |
| (||) Completion held: rework limit reached · KAN-51           |
|                                     [ Allow more attempts ]   |
```

Maps to `AC-OFFICE-COMPLETION-GATES-008.1` to `.8` and `003.2`.

## Tests

| AC | Evidence (work order) |
| --- | --- |
| TASKS-006.1-.9 | Repository and service tests in 01, SQLite and PostgreSQL |
| GATES-001.* | 06 service and handler tests; 08 E2E |
| GATES-002.* | 02 verifier tests with the mock GitHub client |
| GATES-003.* | 02 matching and file-list tests; 05 Inbox; 07 E2E |
| GATES-004.* | 03 decision and verifier tests |
| GATES-005.* | 01 reservation and caller tests; 03 self-approval; 05 extend route |
| GATES-006.* | 04 router tests |
| GATES-007.* | 05 caps, hold, escalation tests |
| GATES-008.* | 07 and 08 unit tests and E2E (chromium and mobile-chrome) |

## E2E tests

- `apps/web/e2e/tests/office/completion-gates-ci.spec.ts` and
  `mobile-completion-gates-ci.spec.ts`: seed a gated task and mock PR checks;
  see fail, pass, protected hold, Allow and Send back (07).
- `apps/web/e2e/tests/office/completion-gates-override.spec.ts` and mobile
  twin: override a blocked completion with a reason (07).
- `apps/web/e2e/tests/office/completion-gates-create.spec.ts` and mobile twin:
  create with criteria; workspace default prefill (08).

## Work orders

- [x] [Task 01: Host-verified criteria in the task gate](task-01-host-verified-criteria.md)
- [x] [Task 02: CI verifier with protected paths](task-02-ci-verifier.md)
- [x] [Task 03: Review pinned to the head](task-03-review-at-head.md)
- [x] [Task 04: Result routing](task-04-result-routing.md)
- [x] [Task 05: Caps, holds and escalation](task-05-caps-holds-escalation.md)
- [x] [Task 06: Creation surfaces and workspace defaults](task-06-creation-surfaces.md)
- [x] [Task 07: Gate status and recovery UI](task-07-gate-status-ui.md)
- [x] [Task 08: Criteria in task creation and settings UI](task-08-creation-ui.md)

## Verification results

All eight work orders are implemented; see each work order's Results. Backend
package tests, sqlguard, race store-conformance and changed-Go lint pass; web
vitest, typecheck, lint and i18n checks pass; chromium and mobile-chrome E2E
for CI, protected paths, override and creation pass. PostgreSQL runs were not
possible (`KANDEV_TEST_POSTGRES_DSN` unset).

The plan stays `in_progress` and the specs stay `draft` because of one open
gap: `AC-OFFICE-COMPLETION-GATES-001.3` asks for a task activity entry when an
inapplicable workspace default is skipped; the implementation logs it instead.
Design changes recorded in the system design: no in-place review re-entry
(a task leaving the review stage with a stale review is returned), keyed
rework counting, and a single extend route for agents and people.

An independent review of the implementation found 13 issues, all fixed with
regression tests: settings updates refuse agent callers; a first pass after a
merge stays `pr_merged_unverified`; `changes_requested` fails the review
criterion and coverage is recomputed from each seat's newest decision; the
all-pass retry only re-applies transitions into a completing step; protected
Allow and Send back require a protected hold at the seen heads, and Send back
has its own control and needs a reason; the public set-criteria command takes
owner and caps confirmation from the caller; rework counting keeps recent keys
and reports the claim; a rejection counts only when the newest review decision
is one; a cap extension releases only the causing cap above use and re-runs the
held return; the CI sweep selects due rows and pages; file-read and
workflow-read errors map to `changed_files_incomplete` and `pending`;
escalation sets `escalated_at` and a sweep retries it, with unescalated cap
holds shown in the Inbox; and `task_completion_sets.state_revision` drives the
task page refresh, which also shows effective caps from a new caps route.

A live run-through on a dev instance (mock GitHub) found and fixed six more
issues, each with a regression test: workflow-engine moves (turn-complete
handoff, reviewer quorum, request changes) publish `task.updated`, not
`task.moved`, so the router now also routes `task.updated` through the
`task_step_transitions` ledger; a gate bounce right after entering Review
could hit the orchestrator's step-entry auto-start on the reviewer's CREATED
Office session, which Office refuses, failing the task (the orchestrator now
leaves CREATED Office sessions to their run); failed tasks were treated as
terminal and stopped routing; running checks with an incomplete file list
spent an attempt; duplicate criterion kinds at creation returned 500; and
malformed settings bodies reported a control error code.

Test-instance review follow-up: **Send back** on a task already in the work
stage counted no round and told nobody, and the next CI evaluation re-held the
same commit. Send back now fails the criterion at the seen heads with the
reason (`sent_back`), counts a round in every stage, wakes the assignee with a
`task_changes_requested` run when no move happens, and the CI verifier keeps
that result until the heads move. The criteria section was reworded with the
operator: the "Weakest" summary and per-row confidence are gone; rows read
**CI** and **Review** with a detail line naming what decided them (failing
checks, protected files, send-back reason, or the approving or
change-requesting reviewers with agent or person avatars) and the commits.

## Risks

- **Check freshness.** The batched GraphQL poller stores only a rollup. The CI
  verifier makes one REST check read per linked PR per head or check change.
  Coalescing per task bounds it; watch GitHub rate limits on large workspaces.
- **Check arrival race.** Checks may appear minutes after a push. The 10-minute
  grace avoids a false `cannot_evaluate`; slow CI providers can still exceed it
  and spend attempts. The cap absorbs this.
- **Review-stage re-entry.** Re-entering the review stage while the task is
  already there depends on step-entry dispatch with a fresh entry identity.
  Task 04 must confirm the mechanism before relying on it.
- **Create-path coverage.** Several repository methods create tasks. Task 01
  must route every one through `insertCompletionCriteriaTx`; a missed path
  creates a task without its criteria.
- **Auth-off human approvals.** Accepted residual risk (ADR), surfaced to the
  user as a configuration-time warning (UI-05, UI-06).
- **Size.** Eight orders across task, Office, GitHub, workflow and web. Keep
  each order's verification scoped to its packages.
