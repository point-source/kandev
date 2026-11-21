# ADR-2026-11-21-completion-gates-people-act-on-exceptions: People act on completion gates only as exceptions

**Status:** proposed
**Date:** 2026-11-21
**Area:** backend

## Context

[ADR-2026-11-20-host-verified-completion-criteria](2026-11-20-host-verified-completion-criteria.md)
made Kandev verify CI and review criteria in process. It also let a person's
task approval pass a review criterion outright, and labelled criteria with
confidence levels (`machine`, `independent_check`, `judgement`) derived from
the criterion kind. Testing showed three problems:

- A person approving a review is a second way to pass Review that skips the
  agent reviewers and makes a person a routine reviewer, while Office seats
  only agents and hides the approval button from people.
- The level names were ambiguous, and a person's decision had no level.
- A review that no agent can ever approve (the only reviewer is the assignee,
  no agent has the reviewer role, or only the author's provider exists)
  waited forever with no signal.

[Issue #5351](https://github.com/kdlbs/kandev/issues/5351) was rewritten
around two rules.

## Decision

1. **People handle exceptions, not routine verification.** Programs and agent
   reviewers decide. A person acts only when they cannot, from the Inbox or the
   task page, never as a reviewer seat or a workflow stage. The review
   criterion counts only decisions from required reviewer seats.
2. **Every person's decision is recorded as what it was.** A person's
   acceptance of one held review (**Accept at this commit**) and a person's
   send back of one criterion (**Send back at this commit**) are heads-bound
   task controls recorded with the person and the reason. They are distinct
   from a whole-task override, which stays on the task page only.

Supporting choices:

- **Kinds and verdict sources.** A criterion is a `check` (a program decides
  by exit status; it names a runner, `github` in phase 1) or a `review`
  (judgement decides). The verdict source is derived, never stored: `program`,
  `person` or `agent`, strongest first. It is recorded in the gate history and
  the API, never shown as a label or summarized per task.
- **Send back routes like a seat rejection.** It counts a rework round,
  returns the task to the work stage and wakes the worker with the reason.
- **Rejecting is always available; accepting only from a hold.** A person can
  send a review back at any time because it fails closed; acceptance needs a
  hold.
- **Holds are named.** Every hold carries a `hold_kind`
  (`protected_files_changed`, `rework_limit_reached`,
  `verification_limit_reached`, `no_reviewer_available`,
  `review_wait_limit_reached`), shows as **Needs you**, and never times out.
- **No reviewer available is split by cause.** Permanent causes hold at once
  and escalate. A seat run parked while another provider is degraded waits,
  visibly, until it resumes or an optional review wait limit runs out.
- **"Limit" replaces "cap"** on every surface.

## Alternatives considered

- **A person's approval counts and gets a button.** Rejected: a second path to
  pass Review that makes a person a routine reviewer and duplicates Override.
- **A human review stage for held tasks.** Rejected: it moves held tasks
  (holds never move tasks), adds a person to agent workflows, and duplicates
  the Inbox.
- **Store a confidence level per criterion.** Rejected: the source follows
  from who verified it, so deriving it cannot drift.

## Consequences

- The review criterion can pass without agents only through a person's
  recorded acceptance of a hold, or through Override.
- With authentication off, an agent's shell calling the API without a run
  token looks identical to the person and can take person actions. The run
  token fence cannot catch it; the task page and the Inbox warn while
  authentication is off.
- Phase 2 adds a Kandev check runner beside `github`, with the same contract
  and no agent between the result and the record.

See the [Office completion gates design](../specs/office/system-design/completion-gates.md)
and the [host-verified criteria amendment](../specs/tasks/requirements/task-completion-host-verified.md).
