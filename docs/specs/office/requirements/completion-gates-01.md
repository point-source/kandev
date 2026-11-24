---
status: active
system: office
created: 2026-11-21
owners:
  - kandev
---

# Office Completion Gates Requirements Part 1

## Overview

This part of the [Office completion gates requirements](completion-gates.md)
covers how people see a gated task and act on it, what happens when no agent
can review it, and the human approval criterion. People act from the Inbox and
the task page, never as a reviewer seat or a workflow stage: on exceptions for
check and review criteria, and routinely only where a human approval criterion
asks them to. Each decision is recorded as what it was. Terms are defined in
the main document.

## Requirements

### REQ-OFFICE-COMPLETION-GATES-008: Gate status and human recovery

**Intent:** Let a person see why a task is or is not Done and act on it from
the Office task page and the Inbox, on desktop and phone.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-008.1:** The Office task page of a gated task
  shall show a **Completion criteria** section listing each criterion with its
  kind and runner, its status (`Pending`, `Passed`, `Failed`, `Cannot
  evaluate`, `Stale`, or **Needs you** for any hold), what decided it (failing
  checks, the hold kind with its labelled value, a send-back reason, a
  person's acceptance, a reviewer wait, or the reviewers who approved or
  requested changes, each marked as an agent or a person), the commits it was
  decided at, and its rework and attempt counts against the limits.
- **AC-OFFICE-COMPLETION-GATES-008.2:** A completed gated task shall say
  **Done**. An overridden completion shall say it was overridden and by whom.
  There is no task-level summary and no verdict-source label on the page.
- **AC-OFFICE-COMPLETION-GATES-008.3:** A person can override a blocked
  completion from the section with a required reason. The override shall bind
  to the current criteria revision and the intended move, as in
  `AC-TASKS-COMPLETION-001.16`. Override is offered on the task page only.
- **AC-OFFICE-COMPLETION-GATES-008.4:** The section and the Inbox item shall
  offer the actions of the hold kind: **Allow for this commit** and **Send
  back** for `protected_files_changed`; **Allow more rounds** for
  `rework_limit_reached`; **Allow more attempts** for
  `verification_limit_reached`; and **Accept at this commit** on any held
  review. The task page also offers **Send back at this commit** on every
  review criterion. The same action in either place shall have the same
  effect, and acting in one place shall resolve the other.
- **AC-OFFICE-COMPLETION-GATES-008.5:** The section shall update without a
  reload when a result, proof, or hold changes. Tasks without criteria shall
  show no section and request no gate data.
- **AC-OFFICE-COMPLETION-GATES-008.6:** On a phone the section and the Inbox
  item shall have touch targets of at least 44 pixels and no horizontal page
  overflow. Dialogs shall open as bottom sheets. All copy shall be localized.
- **AC-OFFICE-COMPLETION-GATES-008.7:** The Office new-task dialog and the
  Office workspace settings shall show the criteria and default editors on
  desktop and phone with the same capabilities.
- **AC-OFFICE-COMPLETION-GATES-008.8:** While authentication is not enabled,
  the criteria section (when it offers actions) and the Inbox (when it lists a
  completion-criteria item) shall show an inline warning that any process on
  this machine can take these actions as the person, with a link to Settings >
  System > Feature Toggles. An agent's shell calling the API without a run
  token looks identical to the person, which is why the warning exists; the
  self-verification fence (`AC-OFFICE-COMPLETION-GATES-005.3`, in the main document) cannot refuse
  it. The warning shall not block any action.
- **AC-OFFICE-COMPLETION-GATES-008.9:** **Accept at this commit** shall verify
  only the held review it is applied to, at the heads the person was shown,
  with a required reason. It shall be refused when the heads moved. It shall
  record the person as the verifier (verdict source `person`) and the reason in
  the gate history as an acceptance, not an override. Every other criterion
  stays in force. Accept shall not move the task; normal completion follows
  once every criterion passes.
- **AC-OFFICE-COMPLETION-GATES-008.10:** The Inbox item of a hold shall be
  titled with the hold kind and its labelled value, as on the task page, and
  link to the task.

### REQ-OFFICE-COMPLETION-GATES-009: No reviewer available

**Intent:** A review no agent can approve must reach a person instead of
waiting forever.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-009.1:** A review criterion shall be refused
  where the task's workflow has no review stage
  (`AC-OFFICE-COMPLETION-GATES-001.3`, in the main document).
- **AC-OFFICE-COMPLETION-GATES-009.2:** When a cause no waiting can fix makes
  every required reviewer seat unable to approve, the review shall hold at once
  with `hold_kind` `no_reviewer_available` and the cause, and escalate through
  the normal path. The causes are: the reviewer seat cast on review-stage entry
  went to the task's assignee (`assignee_only`); no agent can fill the reviewer
  role (`no_reviewer_role`); or a reviewer seat run is parked by the
  author-provider exclusion and the author's provider is the only provider in
  its effective order (`author_provider_only`).
- **AC-OFFICE-COMPLETION-GATES-009.3:** When a reviewer seat run is parked by
  the author-provider exclusion while another provider is degraded with a retry
  time, the review shall stay `Pending` with "Waiting for a reviewer" and the
  park reason, and park-and-resume (`REQ-OFFICE-REVIEW-AUTHOR-EXCL-003`)
  handles it. It shall hold with `review_wait_limit_reached` only when a review
  wait limit is set and the park outlasts it.
- **AC-OFFICE-COMPLETION-GATES-009.4:** When a reviewer seat becomes fillable
  before a person acts, a `no_reviewer_available` or
  `review_wait_limit_reached` hold and a reviewer wait shall clear by
  themselves. A seat becomes fillable when a non-assignee reviewer seat is
  cast, when a parked seat run launches, or when an agent able to review is
  added or activated in the workspace: within a minute Kandev shall seat that
  agent on each review held with `assignee_only` or `no_reviewer_role` while
  the task is at the review stage, start its review, and release the hold. The
  assignee's own seat shall neither pass nor block the review.

### REQ-OFFICE-COMPLETION-GATES-010: Human approval

**Intent:** Let a task require a person's approval of the work as it stands,
on its own or together with the CI check and the agent review, without making
the person a reviewer seat or a workflow stage.

#### Acceptance criteria

- **AC-OFFICE-COMPLETION-GATES-010.1:** Every creation path of
  `AC-OFFICE-COMPLETION-GATES-001.1` and the workspace defaults shall accept a
  human approval criterion (kind `approval`, no runner, an optional rework
  limit). A task shall carry at most one. It can stand alone or sit beside a
  check and a review; it needs a repository but not a review stage. Its
  workflow needs a step after the work stage that does not complete the task;
  otherwise the criterion is refused with
  `completion_criteria_requires_approval_stage`, because it could never ask.
- **AC-OFFICE-COMPLETION-GATES-010.2:** The criterion shall pass only when a
  person approved the current heads. A new head shall make an approval stale
  and ask again. No agent decision, seat decision or evidence submission shall
  pass it.
- **AC-OFFICE-COMPLETION-GATES-010.3:** A person shall be asked only once a
  pull request is linked, the task has left the work stage and every other
  criterion passes at the current heads. Until then the criterion shall show
  `Pending` and name what it waits for. Detaching the last pull request
  withdraws an ask. Asking shall not move the task, hold its assignee or escalate, and the
  ask never times out.
- **AC-OFFICE-COMPLETION-GATES-010.4:** While asked, the task page and an Inbox
  item titled "Approval needed" shall offer **Approve at this commit** (an
  optional note) and **Send back** (a required reason). Approve shall record
  the person as the verifier (verdict source `person`) at the heads shown and
  be refused with `task_completion_evidence_changed` when the heads moved.
  Send back shall route like a review send back
  (`AC-OFFICE-COMPLETION-GATES-004.6`), counting a rework round against this
  criterion's limit. The task page offers Send back at any time once a pull
  request is linked.
- **AC-OFFICE-COMPLETION-GATES-010.5:** Any person who can change the task can
  approve; with authentication off, that is the local person, with the warning
  of `AC-OFFICE-COMPLETION-GATES-008.8`. A request carrying an Office run
  token shall be refused, as for every person action.
- **AC-OFFICE-COMPLETION-GATES-010.6:** Once approved and with every other
  criterion passing, a satisfied quorum-guarded transition into the completing
  step that the gate refused shall be re-applied without another action. A
  transition without such a guard is not replayed, because its step's turn may
  still be running; the person moves the task to its completing step.
- **AC-OFFICE-COMPLETION-GATES-010.7:** The Inbox shall no longer list "your
  review requested" items for the person. Office offers people no seat
  decision, so the human approval item is the person's review request.

## Exclusions

- A person as a reviewer seat or a human review stage. Human approval is a
  criterion (`REQ-OFFICE-COMPLETION-GATES-010`), not a seat or a stage.
- Named approvers or an approval quorum of several people.
- Verdict-source labels or a task-level summary on the task page.

## Related documents

- [Office completion gates requirements](completion-gates.md)
- [System design part 1](../system-design/completion-gates-01.md)
- [Review participant seats](review-participant-seats.md)
- [Review author provider exclusion](review-author-provider-exclusion.md)
