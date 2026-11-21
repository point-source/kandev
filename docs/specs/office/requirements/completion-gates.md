---
status: draft
system: office
created: 2026-11-20
owners:
  - kandev
---

# Office Completion Gates Requirements

## Overview

Office tasks complete on agent decisions alone. A task can reach Done with
failing CI, a reviewer's approval still counts after new commits land, and
nothing stops the worker from changing the tests that judge its own work.

This capability lets an Office task carry two completion criteria that Kandev
checks against the pull request as it stands:

- **CI at head:** the PR's checks at its current head commit succeed, and the
  PR does not touch paths the task creator protected.
- **Review at head:** the reviewers approved the current head commit.

The task system's completion gate (`REQ-TASKS-COMPLETION-001`, `-005` and
`-006`) stores the criteria, refuses every completing move until they pass,
and records the audit history. Office owns which tasks carry criteria, how
Kandev verifies them, what happens when one fails, and how people see and
recover a gated task.

Every gated task ends in one of three states: Done at a stated confidence
level, held with one question for one owner, or cancelled. It never fails
silently or retries without end.

Source: GitHub issue #5351, phase 1. Creator-owned acceptance scripts,
disputes, reviewer quorum across providers, blind review, confidence on
dependents, the reversibility policy and spot-check audits are later phases
and are excluded here.

## Terms

- **Gated task:** an Office task with at least one completion criterion.
- **Head:** the head commit of each pull request linked to the task and not
  detached. A task with several linked PRs has several heads; every criterion
  is judged against all of them.
- **Work stage, review stage:** the first step of the task's workflow with
  `stage_type` `work` or `review`.
- **Rework round:** one return of the task to the work stage because a
  criterion failed.
- **Verification attempt:** one evaluation that ends in `cannot_evaluate` for
  a new observation (a new head or a changed check set).
- **Owner:** the actor that set the criterion. For an agent-created task this
  is the creating agent; for a routine task it is the routine; for a task
  created by a person it is that person.
- **Cap:** the maximum number of verification attempts or rework rounds for one
  criterion. **Ceiling:** the workspace maximum for a cap.

## Requirements

### REQ-OFFICE-COMPLETION-GATES-001: Criteria at task creation

**Intent:** Let whoever creates Office work state what Done means, on every
creation path, and let a workspace require it by default.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-001.1:** The Office new-task dialog, `$KANDEV_CLI
  kandev task create`, the `create_task_kandev` MCP tool, and routine task
  templates shall accept CI-at-head (with optional protected paths and caps)
  and review-at-head criteria. The created task shall carry them from its
  first committed version.
- **AC-OFFICE-COMPLETION-GATES-001.2:** A workspace setting shall define
  default criteria for new Office tasks. When a creation request does not name
  criteria, the default shall apply. A person creating a task can remove a
  default criterion; an agent or a routine cannot.
- **AC-OFFICE-COMPLETION-GATES-001.3:** When a creation request adds a CI-at-head
  or review-at-head criterion to a task with no repository, the system shall
  refuse the creation with `completion_criteria_requires_repository`. When a
  review-at-head criterion is added to a task whose workflow has no review
  stage, the system shall refuse it with
  `completion_criteria_requires_review_stage`. Criteria on a task that is not
  an Office task shall be refused with
  `completion_criteria_requires_office_task`. A workspace default that cannot
  apply to such a task shall be skipped and recorded in the task's activity.
- **AC-OFFICE-COMPLETION-GATES-001.4:** Office tasks without criteria shall
  behave exactly as before this capability, on every completion path.
- **AC-OFFICE-COMPLETION-GATES-001.5:** Protected paths shall be repository-
  relative glob patterns (for example `tests/**`, `.github/workflows/**`). An
  empty or malformed pattern shall be refused at creation with
  `invalid_protected_path`.

### REQ-OFFICE-COMPLETION-GATES-002: Kandev verifies CI at the head

**Intent:** Treat CI as a fact Kandev reads from GitHub, never as a claim an
agent makes.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-002.1:** When every head's checks, chosen by the
  same check selection as PR status, have completed and none failed, and no
  protected path is touched, Kandev shall record `pass` with the heads as
  proof. Skipped and neutral checks shall not count either way.
- **AC-OFFICE-COMPLETION-GATES-002.2:** When any selected check at a head
  ended `failure`, `timed_out` or `action_required`, Kandev shall record
  `fail` with reason `checks_failed` and the failing check names.
- **AC-OFFICE-COMPLETION-GATES-002.3:** While any selected check at a head is
  still running, the criterion shall stay `pending`.
- **AC-OFFICE-COMPLETION-GATES-002.4:** When a head has no selected check, or
  every selected check was skipped or cancelled, for 10 minutes after Kandev
  first observed that head, and no workflow run is active, Kandev shall record
  `cannot_evaluate` with reason `checks_missing`.
- **AC-OFFICE-COMPLETION-GATES-002.5:** When a head moves, Kandev shall treat
  the earlier result as stale and evaluate the new head. A `pass` recorded for
  an earlier head shall never satisfy the gate.
- **AC-OFFICE-COMPLETION-GATES-002.6:** When a linked PR merges before the
  criterion passes, the task shall not complete and shall show the blocker
  `pr_merged_unverified`. A person can override it with a recorded reason.
- **AC-OFFICE-COMPLETION-GATES-002.7:** Kandev shall evaluate after each PR
  sync that changes a head or its checks, and after a backend restart, without
  waiting for a new push.

### REQ-OFFICE-COMPLETION-GATES-003: Protected paths

**Intent:** Stop a PR from weakening the tests or CI that judge it, because
`pull_request` workflows run from the PR's own copy.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-003.1:** When any file the PR adds, modifies,
  deletes, or renames (old or new path) at the current head matches a
  protected path, Kandev shall hold the CI criterion with reason
  `protected_path_changed` and list the matching files. It shall not send the
  task back to the worker.
- **AC-OFFICE-COMPLETION-GATES-003.2:** A protected-path hold shall create one
  Inbox item for people, naming the task, the PR, and the matching files, with
  **Allow for this commit** and **Send back** actions. The item shall not repeat
  for the same heads.
- **AC-OFFICE-COMPLETION-GATES-003.3:** **Allow for this commit** shall record
  the person and the heads, then evaluate CI normally. A new head that still
  touches a protected path shall need a new approval.
- **AC-OFFICE-COMPLETION-GATES-003.4:** **Send back** shall record the person
  and a reason, fail the criterion at those heads with that reason, and count a
  rework round. A task in a later stage returns to the work stage; a task
  already in the work stage stays there and its assignee is woken with the
  reason. The criterion stays failed until the heads move.
- **AC-OFFICE-COMPLETION-GATES-003.5:** When Kandev cannot read the complete
  changed-file list at the current head (for example GitHub's 3000-file limit,
  or a list shorter than the PR's changed-file count), it shall record
  `cannot_evaluate` with reason `changed_files_incomplete`, never `pass`.

### REQ-OFFICE-COMPLETION-GATES-004: Review pinned to the head

**Intent:** Make an approval mean "I reviewed this code", not "I reviewed some
earlier version of this task".

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-004.1:** When a reviewer records `approved` on a
  gated task, the system shall store the heads at that moment with the
  decision.
- **AC-OFFICE-COMPLETION-GATES-004.2:** The review-at-head criterion shall
  pass when every decision-required reviewer seat at the review stage has a
  current `approved` decision whose stored heads equal the current heads.
- **AC-OFFICE-COMPLETION-GATES-004.3:** When a head moves after the criterion
  passed, the criterion shall become stale and the task shall return to the
  review stage, which clears decisions and wakes reviewers again. When the
  task is already in the work stage, it shall stay there.
- **AC-OFFICE-COMPLETION-GATES-004.4:** A person's approval of the task shall
  count as a reviewer approval at the heads it was recorded against, whether
  authentication is on or off.
- **AC-OFFICE-COMPLETION-GATES-004.5:** A reviewer `rejected` decision that
  returns the task to the work stage shall count as one rework round of the
  review-at-head criterion.

### REQ-OFFICE-COMPLETION-GATES-005: The worker cannot verify its own work

**Intent:** Keep the fence intact when authentication is off, where every HTTP
caller is the synthetic admin.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-005.1:** Only Kandev's CI verifier shall record
  a result or proof for a CI-at-head criterion. Any other caller shall be
  refused, whether authentication is on or off (`AC-TASKS-COMPLETION-006.2`).
- **AC-OFFICE-COMPLETION-GATES-005.2:** An `approved` decision from the task's
  assignee agent, or from a run whose agent is the assignee, shall not count
  toward review-at-head. The decision response shall say so with
  `review_self_approval_ignored`.
- **AC-OFFICE-COMPLETION-GATES-005.3:** The assignee agent shall not change a
  criterion, its protected paths, or its caps. Office runtime requests from
  the assignee's runs shall be refused with
  `task_completion_self_verification`.
- **AC-OFFICE-COMPLETION-GATES-005.4:** Every accepted result and proof shall
  record its verifier: `kandev_ci`, the reviewer agent and run, or the person.

### REQ-OFFICE-COMPLETION-GATES-006: Results route the work

**Intent:** Turn each result into the next useful action.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-006.1:** When the CI criterion records `fail`
  and the task is past the work stage, the system shall move the task to the
  work stage, wake the assignee with the failing checks and heads, and count
  one rework round. When the task is already in the work stage, it shall stay
  there and the result shall be visible to the assignee's next run.
- **AC-OFFICE-COMPLETION-GATES-006.2:** A `cannot_evaluate` result shall not
  move the task or wake the worker. It shall count one verification attempt.
- **AC-OFFICE-COMPLETION-GATES-006.3:** When a completing move was refused
  because criteria were unmet, and every criterion later passes while the task
  is still on the step that attempted the move, the system shall attempt the
  workflow's guarded transitions from that step again once.
- **AC-OFFICE-COMPLETION-GATES-006.4:** Repeated delivery of the same PR
  update or decision shall not repeat a move, a wake, or a counter increment.

### REQ-OFFICE-COMPLETION-GATES-007: Caps, hold, and escalate once

**Intent:** Bound the cost of a criterion that keeps failing or cannot be
checked, and hand it to one owner with the history attached.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-007.1:** Each cap shall resolve from the
  criterion, then the task workflow's completing step, then the workspace
  default, then the shipped default of 3. The result shall not exceed the
  workspace ceiling (shipped default 10) unless a person confirmed the higher
  value on the criterion.
- **AC-OFFICE-COMPLETION-GATES-007.2:** A criterion value above the ceiling
  set by an agent or a routine shall be refused with
  `completion_cap_above_ceiling`.
- **AC-OFFICE-COMPLETION-GATES-007.3:** When a `fail` would start a rework
  round beyond the cap, or a `cannot_evaluate` exceeds the attempt cap, the
  system shall hold the criterion instead (`rework_cap_reached` or
  `attempt_cap_reached`). It shall not move the task, and Office shall not
  launch the assignee's runs for that task while the hold lasts.
- **AC-OFFICE-COMPLETION-GATES-007.4:** A cap hold shall escalate once per
  hold: the owner agent is woken when it is active, else the workspace CEO
  agent; a person or a routine without an agent gets one Inbox item. The
  escalation shall carry the result history for the criterion. Repeated
  delivery shall not repeat it.
- **AC-OFFICE-COMPLETION-GATES-007.5:** The owner agent or the CEO agent can
  raise a held criterion's cap within the ceiling through `$KANDEV_CLI kandev
  task gate extend`. A person can raise it above the ceiling from the task
  page or the Inbox item. Raising a cap releases the hold and is recorded.
- **AC-OFFICE-COMPLETION-GATES-007.6:** Existing budget policies shall still
  refuse runs regardless of remaining caps.

### REQ-OFFICE-COMPLETION-GATES-008: Gate status and human recovery

**Intent:** Let a person see why a task is or is not Done and act on it from
the Office task page, on desktop and phone.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-008.1:** The Office task page of a gated task
  shall show a **Completion criteria** section listing each criterion with its
  kind, status (`pending`, `pass`, `fail`, `cannot_evaluate`, `held`, or
  stale), what decided it (failing checks, protected files, a send-back reason,
  or the reviewers who approved or requested changes, each marked as an agent
  or a person), the commits it was decided at, and its rework and attempt
  counts against the caps.
- **AC-OFFICE-COMPLETION-GATES-008.2:** A completed gated task shall say
  **Done**. An overridden completion shall say it was overridden and by whom.
  The confidence level stays in the gate snapshot and summary
  (`AC-TASKS-COMPLETION-006.7`) and is not shown on the page.
- **AC-OFFICE-COMPLETION-GATES-008.3:** A person can override a blocked
  completion from the section with a required reason. The override shall bind
  to the current criteria revision and the intended move, as in
  `AC-TASKS-COMPLETION-001.16`.
- **AC-OFFICE-COMPLETION-GATES-008.4:** The section shall offer **Allow for
  this commit** and **Send back** for a protected-path hold and **Allow more
  attempts** for a cap hold. The same actions on the Inbox item shall have the
  same effect, and acting in one place shall resolve the other.
- **AC-OFFICE-COMPLETION-GATES-008.5:** The section shall update without a
  reload when a result, proof, or hold changes. Tasks without criteria shall
  show no section and request no gate data.
- **AC-OFFICE-COMPLETION-GATES-008.6:** On a phone the section shall render in
  the task's main column with touch targets of at least 44 pixels and no
  horizontal page overflow. Dialogs shall open as bottom sheets. All copy shall
  be localized.
- **AC-OFFICE-COMPLETION-GATES-008.7:** The Office new-task dialog and the
  Office workspace settings shall show the criteria and default editors on
  desktop and phone with the same capabilities.
- **AC-OFFICE-COMPLETION-GATES-008.8:** While authentication is not enabled,
  selecting a review-at-head criterion in the new-task dialog or as a
  workspace default shall show an inline warning. It shall state that
  authentication is off, that any process on this machine can approve the
  task as the user so the criterion can pass without a real review, and that
  turning on authentication in Settings > System > Feature Toggles prevents
  it. The warning shall not block saving or creation; the choice stays with
  the user.

## Exclusions

- Creator-owned acceptance scripts, red-before-green, disputes, reviewer
  quorum across providers, blind review, and structured verdicts (issue #5351
  phase 2).
- Confidence on dependents, the reversibility policy, and spot-check audits
  (phase 3).
- CI and review criteria on non-Office tasks. The task primitive permits them,
  but no surface creates them.
- Branch-protection required-check lists. "CI passes" uses PR-status check
  selection.
- Native gate controls on the generic task page (`REQ-TASKS-COMPLETION-004`).

## Related documents

- [System design](../system-design/completion-gates.md)
- [Task completion requirements](../../tasks/requirements/task-completion.md)
- [Host-verified completion criteria](../../tasks/requirements/task-completion-host-verified.md)
- [Review participant seats](review-participant-seats.md)
- [Review author provider exclusion](review-author-provider-exclusion.md)
- [Blocker terminal policy](blocker-terminal-policy.md)
- [Implementation plan](../../../plans/office-completion-gates/plan.md)
