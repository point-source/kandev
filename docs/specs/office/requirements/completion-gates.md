---
status: active
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

This capability lets an Office task carry completion criteria of three kinds
that Kandev checks against the pull request as it stands:

- **Check:** a program decides, by exit status. Each check names a runner. In
  phase 1 the only runner is `github`: the PR's CI checks at its current head
  commit succeed, and the PR does not touch paths the task creator protected.
- **Review:** judgement decides. The required reviewer seats approved the
  current head commit.
- **Approval:** a person decides. A person approved the current head commit
  (`REQ-OFFICE-COMPLETION-GATES-010`, in part 1).

The task system's completion gate (`REQ-TASKS-COMPLETION-001`, `-005` and
`-006`) stores the criteria, refuses every completing move until they pass,
and records the audit history. Office owns which tasks carry criteria, how
Kandev verifies them, what happens when one fails, and how people see and
recover a gated task.

Programs and agent reviewers decide checks and reviews; a person acts on
those only when they cannot, from the Inbox or the task page, never as a seat
or a workflow stage. A task that needs a person's judgement carries a human
approval criterion, alone or with the others. Every person's decision is
recorded as what it was, because these records later judge how reliable each
kind of verifier is.

Every gated task ends in one of three states: Done, held with one question
for one owner, or cancelled. It never fails silently or retries without end.

Source: GitHub issue #5351, phase 1. The Kandev check runner, disputes,
reviewer quorum across providers, blind review, structured verdicts, the
reversibility policy and spot-check audits are later phases and are excluded
here.

## Terms

- **Gated task:** an Office task with at least one completion criterion.
- **Head:** the head commit of each pull request linked to the task and not
  detached. A task with several linked PRs has several heads; every criterion
  is judged against all of them.
- **Work stage, review stage:** the first step of the task's workflow with
  `stage_type` `work` or `review`.
- **Required reviewer seat:** a decision-required `reviewer` participant seat
  at the review stage.
- **Rework round:** one return of the task to the work stage because a
  criterion failed.
- **Verification attempt:** one evaluation that ends in `cannot_evaluate` for
  a new observation (a new head or a changed check set).
- **Owner:** the actor that set the criterion. For an agent-created task this
  is the creating agent; for a routine task it is the routine; for a task
  created by a person it is that person.
- **Limit:** the most rework rounds (`rework_limit`) or verification attempts
  (`verification_limit`) for one criterion, or how long a parked reviewer may
  wait (`review_wait_limit`, optional). **Ceiling:** the workspace maximum for
  a counted limit.
- **Hold:** automation has stopped on a criterion and waits for a person: the
  assignee is not woken and the task does not move. A hold has a `hold_kind`
  and never times out.
- **Verdict source:** what produced a verified criterion's verdict, derived and
  never stored: `program`, `person` or `agent`
  (`AC-TASKS-COMPLETION-006.7`).

## Requirements

### REQ-OFFICE-COMPLETION-GATES-001: Criteria at task creation

**Intent:** Let whoever creates Office work state what Done means, on every
creation path, and let a workspace require it by default.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-001.1:** The Office new-task dialog, `$KANDEV_CLI
  kandev task create`, the `create_task_kandev` MCP tool, and routine task
  templates shall accept a check criterion naming the `github` runner (with
  optional protected paths and limits), a review criterion (with optional
  limits, including a review wait limit), and a human approval criterion
  (`AC-OFFICE-COMPLETION-GATES-010.1`). A check without a runner, a check
  naming another runner, or a review or approval naming a runner shall be
  refused. A task shall carry at most one criterion of each kind; a second one
  shall be refused at creation, when the criteria are edited, and through
  the plugin write path. The created task shall carry the criteria from its
  first committed version.
- **AC-OFFICE-COMPLETION-GATES-001.2:** A workspace setting shall define
  default criteria for new Office tasks. When a creation request does not name
  criteria, the default shall apply. A person creating a task can remove a
  default criterion; an agent or a routine cannot.
- **AC-OFFICE-COMPLETION-GATES-001.3:** When a creation request adds a check
  or review criterion to a task with no repository, the system shall refuse
  the creation with `completion_criteria_requires_repository`. A review
  criterion on a task whose workflow has no review stage shall be refused with
  `completion_criteria_requires_review_stage`, at creation, when the criteria
  are edited, and through the plugin write path. Criteria on a task that is
  not an Office task shall be refused with
  `completion_criteria_requires_office_task`. A workspace default that cannot
  apply to such a task shall be skipped and recorded in the task's activity.
- **AC-OFFICE-COMPLETION-GATES-001.4:** Office tasks without criteria shall
  behave exactly as before this capability, on every completion path.
- **AC-OFFICE-COMPLETION-GATES-001.5:** Protected paths shall be repository-
  relative glob patterns (for example `tests/**`, `.github/workflows/**`). An
  empty or malformed pattern shall be refused at creation with
  `invalid_protected_path`.

### REQ-OFFICE-COMPLETION-GATES-002: Kandev verifies the check at the head

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
`pull_request` workflows run from the PR's own copy. Until the Kandev check
runner exists, an acceptance check is a CI job whose workflow and script sit
under protected paths.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-003.1:** When any file the PR adds, modifies,
  deletes, or renames (old or new path) at the current head matches a
  protected path, Kandev shall hold the check with `hold_kind`
  `protected_files_changed` and list the matching files. It shall not send the
  task back to the worker.
- **AC-OFFICE-COMPLETION-GATES-003.2:** A protected-files hold shall create one
  Inbox item for people, naming the hold, the matching files and the task,
  with **Allow for this commit** and **Send back** actions. The item shall not
  repeat for the same heads.
- **AC-OFFICE-COMPLETION-GATES-003.3:** **Allow for this commit** shall record
  the person and the heads, then evaluate the check normally. A new head that
  still touches a protected path shall need a new approval.
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
- **AC-OFFICE-COMPLETION-GATES-004.2:** The review criterion shall count only
  decisions from required reviewer seats. It shall pass when every required
  reviewer seat has a current `approved` decision whose stored heads equal the
  current heads, and fail when any of them has a current rejection. A person's
  decision on the task is not a seat decision and shall not count.
- **AC-OFFICE-COMPLETION-GATES-004.3:** When a head moves after the criterion
  passed, the criterion shall become stale and the task shall return to the
  review stage, which clears decisions and wakes reviewers again. When the
  task is already in the work stage, it shall stay there.
- **AC-OFFICE-COMPLETION-GATES-004.5:** A reviewer rejection that returns the
  task to the work stage shall count as one rework round of the review
  criterion.
- **AC-OFFICE-COMPLETION-GATES-004.6:** A person can **Send back at this
  commit** on a review criterion from the task page at any time, with a
  required reason. It shall be refused when the heads moved since the page
  showed them. It shall fail the criterion at those heads, record the person,
  and route exactly like a seat rejection: count one rework round, return the
  task to the work stage, and wake the worker with the reason. Approvals at the
  same heads shall not override it; the criterion stays failed until the heads
  move. Reaching the rework limit this way holds the criterion as usual.

### REQ-OFFICE-COMPLETION-GATES-005: The worker cannot verify its own work

**Intent:** Keep the fence intact when authentication is off, where every HTTP
caller is the synthetic admin.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-005.1:** Only Kandev's check verifier shall
  record a result or proof for a check criterion. Any other caller shall be
  refused, whether authentication is on or off (`AC-TASKS-COMPLETION-006.2`).
- **AC-OFFICE-COMPLETION-GATES-005.2:** An `approved` decision from the task's
  assignee agent, or from a run whose agent is the assignee, shall not count
  toward the review criterion. The decision response shall say so with
  `review_self_approval_ignored`.
- **AC-OFFICE-COMPLETION-GATES-005.3:** The assignee agent shall not change a
  criterion, its protected paths, or its limits. Office runtime requests from
  the assignee's runs shall be refused with
  `task_completion_self_verification`. A person's gate actions (Allow, Send
  back, Accept, Allow more rounds or attempts) shall be refused for a request
  carrying an Office run token, with authentication on or off.
- **AC-OFFICE-COMPLETION-GATES-005.4:** Every accepted result and proof shall
  record its verifier: `kandev_ci`, the required reviewer seats, or the
  person who accepted it.
- **AC-OFFICE-COMPLETION-GATES-005.5:** The gate history and the API shall
  carry each verified criterion's verdict source: `program` for a check,
  `agent` for an agent reviewer's verdict, and `person` for a person's
  acceptance. The verdict source shall not be shown on the task page.

### REQ-OFFICE-COMPLETION-GATES-006: Results route the work

**Intent:** Turn each result into the next useful action.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-006.1:** When the check records `fail` and the
  task is past the work stage, the system shall move the task to the work
  stage, wake the assignee with the failing checks and heads, and count one
  rework round. When the task is already in the work stage, it shall stay there
  and the result shall be visible to the assignee's next run.
- **AC-OFFICE-COMPLETION-GATES-006.2:** A `cannot_evaluate` result shall not
  move the task or wake the worker. It shall count one verification attempt.
- **AC-OFFICE-COMPLETION-GATES-006.3:** When a completing move was refused
  because criteria were unmet, and every criterion later passes while the task
  is still on the step that attempted the move, the system shall attempt the
  workflow's guarded transitions from that step again once.
- **AC-OFFICE-COMPLETION-GATES-006.4:** Repeated delivery of the same PR
  update or decision shall not repeat a move, a wake, or a counter increment.

### REQ-OFFICE-COMPLETION-GATES-007: Limits, holds, and escalate once

**Intent:** Bound the cost of a criterion that keeps failing or cannot be
checked, and hand it to one owner with the history attached.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-007.1:** Each counted limit shall resolve from
  the criterion, then the task workflow's completing step, then the workspace
  default, then the shipped default of 3. The result shall not exceed the
  workspace ceiling (shipped default 10) unless a person confirmed the higher
  value on the criterion. The review wait limit shall resolve from the
  criterion, then the step, then the workspace; it is unset by default, which
  means a parked review waits indefinitely, and it has no ceiling.
- **AC-OFFICE-COMPLETION-GATES-007.2:** A criterion value above the ceiling
  set by an agent or a routine shall be refused with
  `completion_limit_above_ceiling`.
- **AC-OFFICE-COMPLETION-GATES-007.3:** When a `fail` would start a rework
  round beyond the limit, or a `cannot_evaluate` exceeds the verification
  limit, the system shall hold the criterion instead (`rework_limit_reached` or
  `verification_limit_reached`). It shall not move the task, and Office shall
  not launch the assignee's runs for that task while the hold lasts.
- **AC-OFFICE-COMPLETION-GATES-007.4:** A hold other than
  `protected_files_changed` shall escalate once per hold: the owner agent is
  woken when it is active, else the workspace CEO agent; a person or a routine
  without an agent gets one Inbox item. The escalation shall carry the result
  history for the criterion and name what the owner can do for that hold kind.
  Repeated delivery shall not repeat it.
- **AC-OFFICE-COMPLETION-GATES-007.5:** The owner agent or the CEO agent can
  raise a held criterion's limit within the ceiling through `$KANDEV_CLI kandev
  task gate extend` (`--rework-limit`, `--verification-limit`). A person can
  raise it above the ceiling from the task page or the Inbox item. Raising a
  limit releases the hold and is recorded.
- **AC-OFFICE-COMPLETION-GATES-007.6:** Existing budget policies shall still
  refuse runs regardless of remaining limits.
- **AC-OFFICE-COMPLETION-GATES-007.7:** Every hold shall carry one `hold_kind`
  in the API: `protected_files_changed`, `rework_limit_reached`,
  `verification_limit_reached`, `no_reviewer_available`, or
  `review_wait_limit_reached`. No hold shall time out; reaching a limit only
  decides when a person is asked.

Gate status, people's actions, and what happens when no reviewer is available
(`REQ-OFFICE-COMPLETION-GATES-008` and `-009`) are in
[part 1](completion-gates-01.md).

## Exclusions

- The Kandev check runner and check profiles with red-before-green, disputes,
  reviewer quorum across providers, blind review, and structured verdicts
  (issue #5351 phase 2). The phase 2 runner design lives in the system design.
- The reversibility policy and spot-check audits (phase 3).
- Check and review criteria on non-Office tasks. The task primitive permits
  them, but no surface creates them.
- Branch-protection required-check lists. "CI passes" uses PR-status check
  selection.
- Native gate controls on the generic task page (`REQ-TASKS-COMPLETION-004`).
- A person as a reviewer seat or a human review stage.

## Related documents

- [Part 1: status, people's actions, reviewer availability](completion-gates-01.md)
- [System design](../system-design/completion-gates.md)
- [Task completion requirements](../../tasks/requirements/task-completion.md)
- [Host-verified completion criteria](../../tasks/requirements/task-completion-host-verified.md)
- [Review participant seats](review-participant-seats.md)
- [Review author provider exclusion](review-author-provider-exclusion.md)
- [Blocker terminal policy](blocker-terminal-policy.md)
- [Implementation plan](../../../plans/office-completion-gates/plan.md)
- [Decision: people act on exceptions](../../../decisions/2026-11-21-completion-gates-people-act-on-exceptions.md)
