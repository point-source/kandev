---
status: active
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
(`REQ-OFFICE-COMPLETION-GATES-001` to `-009`) are the first consumer.

#### Terms

- **Criterion kind:** `attested` (every criterion that exists before this
  amendment, verified by any authorized caller), `check` (a program decides by
  exit status; it names a runner, `github` in phase 1), `review` (judgement
  decides) or `approval` (a person decides). The last three are
  **host-verified kinds**.
- **PR heads subject:** the set of head commits of every pull request linked
  to the task and not detached, as one revision.
- **Verdict source:** what produced a verified criterion's verdict, derived
  and never stored: `person` when a person verified it (verifier kind
  `human`, always for `approval`), otherwise `program` for a `check` and
  `agent` for `review` and `attested`. Order from strongest: `program`, `person`, `agent`. An
  unverified criterion has no verdict source. Separation from the worker is a
  rule on every source (`AC-TASKS-COMPLETION-006.8`), not a source.

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
  actor that set it), optional limits for verification attempts and rework
  rounds, and its counters for both. Changing the criterion's meaning shall
  reset its counters and its result to `pending`.
- **AC-TASKS-COMPLETION-006.6:** A criterion can be held with a hold kind
  named by the owning system. A held criterion shall stay unmet until a human
  or the owning system releases it or records a person's result for it. Each
  hold and release shall be recorded in the gate history.
- **AC-TASKS-COMPLETION-006.7:** The gate snapshot shall report the verdict
  source of each verified criterion, and the gate history shall record it with
  each result. Neither the snapshot nor the compact summary carries a
  task-level aggregate such as a weakest source.
- **AC-TASKS-COMPLETION-006.10:** The owning system can record a person's
  result for a host-verified criterion through a task control bound to the
  heads the person saw: an acceptance of a held criterion or an approval of
  an `approval` criterion (`pass`, verifier kind `human`), or a send back
  (`fail` with the person's reason). A control applied after the
  heads moved shall be refused with `task_completion_evidence_changed`. This is
  a control, not evidence: `AC-TASKS-COMPLETION-006.2` still refuses evidence
  for host-verified kinds.
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

- Verifier policy, routing, limits resolution, and UI belong to the consumer;
  for Office see [Office completion gates](../../office/requirements/completion-gates.md).
- No HTTP route records a host-verified result as evidence. A person's
  result reaches the gate only through the owning system's control
  (`AC-TASKS-COMPLETION-006.10`).
- The Office consumer's reasoning for people acting only on exceptions is in
  [the November 21 decision](../../../decisions/2026-11-21-completion-gates-people-act-on-exceptions.md).

## Related documents

- [Task completion requirements](task-completion.md)
- [Coordination controls design](../system-design/coordination-controls.md)
- [Implementation plan](../../../plans/office-completion-gates/plan.md)
