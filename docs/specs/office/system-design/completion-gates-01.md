---
status: draft
system: office
requirements:
  - REQ-OFFICE-COMPLETION-GATES-008
  - REQ-OFFICE-COMPLETION-GATES-009
created: 2026-11-21
---

# Office Completion Gates System Design Part 1

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-COMPLETION-GATES-008` | [Person actions](#person-actions), [User interface](#user-interface) |
| `REQ-OFFICE-COMPLETION-GATES-009` | [No reviewer available](#no-reviewer-available) |

## Purpose and boundaries

This part of the [Office completion gates design](completion-gates.md) covers
the holds raised when no reviewer can approve, the person actions, the Office
user interface, and the phase 2 check runner. The task-owned primitive, the
verifiers, routing and limits are in the main design.

## No reviewer available

`completiongates/reviewers.go` holds a review no agent can approve and clears
it when a reviewer can run again. Holds are applied with the `hold` control at
the live heads, so a person acts on the commit they are shown, and a new hold
escalates once through the normal path.

- **Seat cast on review-stage entry:** Office's
  `engine_adapters.SeatCasterAdapter` reports every reviewer-role cast to a
  `ReviewerCastObserver` (`Service.ReviewerCast`). An unfillable cast holds
  with cause `no_reviewer_role`; a cast that seats the task's assignee holds
  with cause `assignee_only`; any other cast releases those holds. The cast is
  synchronous inside the step entry, so it is the one point that knows the
  seat without racing the seat write.
- **Seat run parked by the author-provider exclusion:** the scheduler reports
  parks of seat runs the exclusion applies to through a `ReviewSeatObserver`
  (`Service.ReviewSeatParked`). When the author's provider is the only
  provider in the resolution's effective order, the review holds at once with
  cause `author_provider_only`. Otherwise the review records `pending` /
  `waiting_for_reviewer` with detail `[park status, earliest retry, wait start]`
  and park-and-resume handles the run.
- **Seat run launched:** `Service.ReviewSeatLaunched` releases a
  `no_reviewer_available` or `review_wait_limit_reached` hold and clears a
  reviewer wait.
- **Review wait limit:** the escalation loop's one-minute tick runs
  `sweepReviewWaits`, which holds a review with `review_wait_limit_reached`
  once a reviewer wait has lasted its resolved review wait limit. Without a
  limit the review waits.

## Person actions

People act on exceptions only, from the task page and the Inbox. Every person
action requires a person identity, is refused for an Office run token, carries
the criterion revision and the heads the person saw, and records the person as
the history actor with their reason.

| Route (`POST /api/v1/office/tasks/:id/completion-gate/criteria/:criterionId/...`) | Control | Applies to |
| --- | --- | --- |
| `allow` | `allow_protected_paths` | A check held with `protected_files_changed` |
| `send-back` | `send_back` | A check held with `protected_files_changed`; a review at any time |
| `accept` | `accept` | A held review |
| `extend` | `extend_limits` | A limit hold (also the agent route, within the ceiling) |

The body is `{expected_criterion_revision, expected_subject_revision, reason}`.
A stale revision or heads returns 409 `task_completion_evidence_changed`.
**Accept** needs a reason, records `pass` / `accepted` at the heads shown with
verifier kind `human` and the person as verifier (verdict source `person`),
clears the hold, and leaves every other criterion as it is. **Send back**
needs a reason, records `fail` / `sent_back` at the heads shown, clears any
hold and verification, and routes as described in
[Result routing](completion-gates.md#result-routing). **Override completion** stays the existing
whole-task move override on the task page only.

## User interface

The Office task page (`components/task/simple/OfficeSimplePane.tsx`) renders a
new `CompletionCriteriaSection` in the main column after `TaskActionRow`,
only when the task status summary reports `criteria_count > 0`. It reads `GET
/api/v1/tasks/:id/completion-gate` and the effective limits, and refetches when
the status summary's gate `revision` or `state_revision` changes. Every gate
write (criteria set, evidence, result, hold, control) bumps `state_revision` on
`task_completion_sets`, and the batch and standalone summaries both carry it.

The header shows the title only, plus **Done** or **Done by override · actor**
once the task is done. Each row shows the kind and runner (**CI check
(GitHub)**, **Review**), the status badge (**Needs you** for every hold), a
detail line, and the rework and attempt counts against the limits. The detail
line (`lib/completion-gates/criterion-detail.ts`) names what decided the
result, then the short heads (7 characters per PR) it was decided at, or the
live heads when there is no result yet:

- A hold names its kind with its labelled value: the protected files; the
  rounds or attempts used of the limit; the no-reviewer cause; or how long the
  review waited.
- A reviewer wait reads "Waiting for a reviewer" with the retry time when
  known; it is `Pending`, not a hold.
- A check shows failing check names or a send-back reason; a review shows a
  send-back reason, a person's acceptance with its reason, **Needs approval**,
  or the reviewers who approved (decision IDs in `evidence.reference`) or
  requested changes, each with the agent avatar or the person icon.

Actions come from one helper (`criterionActions`) shared by the task page and
the Inbox, and apply through one client (`lib/completion-gates/person-actions.ts`):
**Allow for this commit** and **Send back** for a protected-files hold;
**Allow more rounds** or **Allow more attempts** for a limit hold (the dialog
asks for that limit only); **Accept at this commit** on a held review; and, on
the task page only, **Send back at this commit** on every review. Dialogs open
as bottom sheets on phones. **Override completion** stays at the bottom of the
section.

The Inbox renders a `completion_gate_hold` item with the same detail line as
its title, a link to the task, and the same actions.

While `state.auth.mode` is not `enabled`, the section (when it offers actions)
and the Inbox (when it lists a completion-criteria item) render a non-blocking
warning with a link to the Feature Toggles page
(`AC-OFFICE-COMPLETION-GATES-008.8`). It reads only existing boot state.

The new-task dialog (`app/office/components/new-task-dialog.tsx`) gets a
**Completion criteria** group prefilled from workspace settings, with the
review wait limit on a review. Workspace settings
(`app/office/workspace/settings/components/settings-content.tsx`) get a
**Completion gates** card for defaults (including the default review wait
limit), limits and ceilings. API clients live in
`lib/api/domains/task-completion-gates-api.ts`. All copy is in seven locales.

The phone composition follows the existing Office task page: one scrolling
main column; the section and the Inbox item use full-width rows and 44-pixel
action targets.

## Phase 2: Kandev check runner

Not implemented in phase 1; recorded so phase 1 stays consistent with it. A
check criterion names its runner, so the Kandev runner is a second runner
beside `github`, and the github runner may later become one implementation of
a shared runner abstraction. Phase 1 acceptance checks are CI jobs under
protected paths.

- **Executor pattern:** `ExecutorBackend.CreateInstance` already starts an
  agentctl instance without an agent subprocess, and agentctl exposes `POST
  /api/v1/processes/start`. The runner creates a fresh instance from a **check
  profile** (an executor profile with no agent configuration and with check
  constraints), checks out the pinned commit (never the worker's worktree),
  writes the creator's versioned script stored in Kandev, runs it, records the
  exit code, and stops the instance.
- **Contract:** exit 0 is `pass`, 1 is `fail`, a reserved code (2) is
  `cannot_evaluate`, and any other exit, a timeout or a crash is
  `cannot_evaluate`. An optional size-limited result file; logs stored as an
  artifact. Declared timeout, image and network access (off by default);
  secrets from an allowlist, empty by default.
- **Guarantees:** no agent, Kandev credentials or MCP servers in the instance;
  each result pinned to the definition digest, the commit and the image digest;
  red-before-green against the base branch when the criterion is set; isolated
  executors only.

## Related documents

- [Office completion gates design](completion-gates.md)
- [Requirements part 1](../requirements/completion-gates-01.md)
- [ADR: People act on exceptions](../../../decisions/2026-11-21-completion-gates-people-act-on-exceptions.md)
