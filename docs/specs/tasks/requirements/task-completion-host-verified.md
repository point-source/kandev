---
status: draft
system: tasks
created: 2026-11-20
owners:
  - kandev
---

# Host-Verified Completion Criteria Requirements

## Overview

This amendment to [task completion](task-completion.md) adds criteria that
Kandev itself verifies from a system of record. It extends the optional
evidence gate (`AC-TASKS-COMPLETION-001.14` to `.16`) and keeps its guard,
override, and audit rules.

## Requirements

### REQ-TASKS-COMPLETION-006: Host-verified criteria and evaluation results

**Intent:** Let Kandev itself verify a completion criterion from a system of
record, and record why a criterion is not yet met, without letting any other
caller vouch for it. Office completion gates
(`REQ-OFFICE-COMPLETION-GATES-001` to `-008`) are the first consumer.

#### Terms

- **Criterion kind:** `attested` (every criterion that exists before this
  amendment, verified by any authorized caller), `ci_at_head` or
  `review_at_head`. The last two are **host-verified kinds**.
- **PR heads subject:** the set of head commits of every pull request linked
  to the task and not detached, as one revision.
- **Confidence level:** `machine` for `ci_at_head`, `judgement` for
  `review_at_head` and `attested`. Order from strongest: `machine`,
  `independent_check`, `judgement`. `independent_check` is reserved for a later
  criterion kind.

#### Acceptance criteria

- **AC-TASKS-COMPLETION-006.1:** A criterion shall carry a kind. Criteria
  stored before this amendment, and criteria written without a kind, shall be
  `attested` and shall keep their current behavior.
- **AC-TASKS-COMPLETION-006.2:** When an HTTP, plugin Host, or MCP caller
  submits evidence for a host-verified criterion, the system shall refuse it
  with `task_completion_verifier_reserved` and leave the criterion unchanged.
  This shall hold whether authentication is on or off.
- **AC-TASKS-COMPLETION-006.3:** A host-verified criterion shall use the PR
  heads subject. Pushing a new head, linking another pull request, or
  detaching one shall make its proof stale. When the task has no linked pull
  request, the gate shall report the blocker `pr_missing`.
- **AC-TASKS-COMPLETION-006.4:** Each criterion shall expose its latest
  evaluation result: `pending`, `pass`, `fail` or `cannot_evaluate`, with a
  bounded reason code and the subject revision it observed. Only `pass` with
  current proof shall satisfy the gate. Each change of result shall be kept in
  the gate history with its actor.
- **AC-TASKS-COMPLETION-006.5:** A criterion shall record its owner (the
  actor that set it), optional caps for verification attempts and rework
  rounds, and its counters for both. Changing the criterion's meaning shall
  reset its counters and its result to `pending`.
- **AC-TASKS-COMPLETION-006.6:** A criterion can be marked `held` with a
  reason. A held criterion shall stay unmet until a human or the owning
  system releases it. Release shall be recorded in the gate history.
- **AC-TASKS-COMPLETION-006.7:** The gate snapshot and the compact gate
  summary shall report the confidence level of each criterion and the weakest
  level among the task's criteria.
- **AC-TASKS-COMPLETION-006.8:** Setting criteria from a request
  authenticated as one of the task's own Office runs, or from the task's
  assignee agent, shall be refused with `task_completion_self_verification`.
  Setting criteria in the same transaction that creates the task shall be
  permitted for the creator.
- **AC-TASKS-COMPLETION-006.9:** Criteria supplied when a task is created
  shall be stored in the same transaction as the task. A refused criterion set
  shall refuse the task creation.

#### Office task detail amendment (November 20)

This amendment narrows `AC-TASKS-COMPLETION-004.1`. The generic task page keeps
no native gate controls. The Office task page can present gate status and the
human recovery actions that `REQ-OFFICE-COMPLETION-GATES-008` defines.


## Exclusions

- Verifier policy, routing, caps resolution, and UI belong to the consumer;
  for Office see [Office completion gates](../../office/requirements/completion-gates.md).
- No HTTP route records a host-verified result.

## Related documents

- [Task completion requirements](task-completion.md)
- [Coordination controls design](../system-design/coordination-controls.md)
- [Implementation plan](../../../plans/office-completion-gates/plan.md)
