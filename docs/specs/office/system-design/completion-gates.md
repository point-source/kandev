---
status: draft
system: office
requirements:
  - REQ-OFFICE-COMPLETION-GATES-001
  - REQ-OFFICE-COMPLETION-GATES-002
  - REQ-OFFICE-COMPLETION-GATES-003
  - REQ-OFFICE-COMPLETION-GATES-004
  - REQ-OFFICE-COMPLETION-GATES-005
  - REQ-OFFICE-COMPLETION-GATES-006
  - REQ-OFFICE-COMPLETION-GATES-007
  - REQ-OFFICE-COMPLETION-GATES-008
created: 2026-11-20
---

# Office Completion Gates System Design

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-COMPLETION-GATES-001` | [Creation surfaces and workspace defaults](#creation-surfaces-and-workspace-defaults) |
| `REQ-OFFICE-COMPLETION-GATES-002` | [CI verifier](#ci-verifier) |
| `REQ-OFFICE-COMPLETION-GATES-003` | [Protected paths](#protected-paths) |
| `REQ-OFFICE-COMPLETION-GATES-004` | [Review at head](#review-at-head) |
| `REQ-OFFICE-COMPLETION-GATES-005` | [Fences](#fences) |
| `REQ-OFFICE-COMPLETION-GATES-006` | [Result routing](#result-routing) |
| `REQ-OFFICE-COMPLETION-GATES-007` | [Caps, holds and escalation](#caps-holds-and-escalation) |
| `REQ-OFFICE-COMPLETION-GATES-008` | [User interface](#user-interface) |

## Purpose and boundaries

The task system owns the gate primitive: criteria, evidence, results, holds,
counters, the completing-move guard, overrides and history. Its extension for
this feature is `REQ-TASKS-COMPLETION-006`, designed in
[Task-owned primitive](#task-owned-primitive) below and in the
[coordination controls design](../../tasks/system-design/coordination-controls.md#completion-gates).

Office owns policy: which tasks carry criteria, the CI and review verifiers,
what a result does to the workflow, cap resolution, escalation, and the Office
UI. Office code lives in a new package `internal/office/completiongates`. It
writes to the gate only through task-service commands. It never writes gate
tables directly.

GitHub owns PR state. The CI verifier reads it through a narrow interface on
`internal/github.Service`; it does not call GitHub clients itself.

The gate is still evaluated from stored state at the completing commit
(`guardTaskCompletionTransitionTx`). No verifier runs inside a task
transaction, and an unavailable verifier leaves a visible blocker, never a
deadlock.

## Task-owned primitive

### Schema

`task_completion_criteria` gains columns through idempotent `ADD COLUMN`
migrations in `base_migrations.go` (SQLite and PostgreSQL):

| Column | Meaning |
| --- | --- |
| `kind` | `attested` (default), `ci_at_head`, `review_at_head` |
| `config_json` | `{"protected_paths": [...], "max_verification_attempts": n, "max_rework_rounds": n, "caps_confirmed_by": "<user>"}` |
| `owner_kind`, `owner_id` | `user`, `agent`, `routine`, or `plugin`, and its ID |
| `result`, `result_reason`, `result_detail_json` | Latest evaluation (`AC-TASKS-COMPLETION-006.4`); detail is bounded (at most 20 names, 200 bytes each) |
| `result_subject_revision`, `result_first_seen_at`, `result_at` | The PR heads the result observed and when Kandev first saw them |
| `attempt_count`, `rework_count` | Counters (`AC-TASKS-COMPLETION-006.5`) |
| `held_reason`, `held_at` | Hold (`AC-TASKS-COMPLETION-006.6`); empty when not held |
| `protected_allowed_revision`, `protected_allowed_by` | Person approval of a protected-path change at one heads revision |

`criterion_revision` increments when `kind`, `description`, or
`config_json.protected_paths` changes. That reset zeroes counters, clears the
hold and the protected approval, and sets `result` to `pending`
(`completionCriterionEquivalent` gains `kind` and `protected_paths`). A cap
change alone does not change the revision.

### PR heads subject

New subject kind `github_pr_heads` in `task/models/completion_gates.go`.
`ID` is the task ID. `Revision` is the canonical string of every linked,
non-detached PR: `<github_task_prs.id>@<head_sha>` sorted by ID and joined by
commas. One shared function builds it from rows; Go and the batch SQL reader
(`completion_gate_batch.go`) both use it (the SQL reader selects the rows and
calls the Go function).

`evidenceSubjectCurrentTx` recomputes the revision from `github_task_prs WHERE
task_id = ? AND detached_at IS NULL` (row lock on PostgreSQL) and compares it.
Host-verified criteria always use this subject; `validEvidenceSubject` refuses
other subjects for them.

New blocker reasons beside `unverified` and `evidence_stale`: `pr_missing`
(no linked PR), `pr_merged_unverified` (not passed and a linked row has
`state = merged`), and `held`.

### Commands

All are task-service methods. None has an HTTP route of its own.

- `RecordTaskCompletionEvaluation(ctx, HostEvaluation)` writes a result for a
  host-verified criterion. It takes the expected criterion revision and the
  observed subject revision, and applies only when both still match the live
  rows inside the transaction. `pass` also writes evidence, `verified_revision`
  and the verifier. It increments `attempt_count` or `rework_count` when asked,
  sets or clears the hold, and appends history. It is idempotent on
  `(criterion_id, criterion_revision, subject_revision, result, reason)`: a
  repeat returns `changed=false` and counts nothing.
- `ReleaseTaskCompletionHold`, `ExtendTaskCompletionCaps` and
  `AllowTaskCompletionProtectedPaths` take an actor and a reason and append
  history.
- `VerifyTaskCompletionCriterion`, its Exact variant, and the plugin Host
  method (`plugins/host_completion_gates.go`) refuse host-verified criteria
  with `repoerrors.ErrTaskCompletionVerifierReserved`
  (`task_completion_verifier_reserved`).

The service passes a `models.CompletionGateCaller` read from the context
(`WithCompletionGateCaller`, the same pattern as
`WithTaskCompletionMoveOverride`). Office runtime handlers set it from the run
JWT (`RunContext`). `SetTaskCompletionCriteria` refuses a caller whose agent is
the task assignee or whose run belongs to the task's assignee with
`ErrTaskCompletionSelfVerification` (`AC-TASKS-COMPLETION-006.8`).

### Atomic creation

`CreateTaskRequest` gains `CompletionCriteria []CompletionCriterionInput`.
Each task-creating repository method calls one shared
`insertCompletionCriteriaTx` before commit, so criteria exist from the first
committed task version (`AC-TASKS-COMPLETION-006.9`). Validation failure
returns a validation error and nothing is written.

### Confidence

Confidence is derived from `kind`, never stored. The snapshot adds
`confidence` to each criterion and `weakest_confidence` to the gate.
`TaskCompletionGateSummaryObservation` adds `weakest_confidence` and
`held_count`; the task status summary carries them so lists show gate state
without detail reads.

## Creation surfaces and workspace defaults

| Surface | Change |
| --- | --- |
| Office new-task dialog | `POST /api/v1/tasks` body gains `completion_criteria` (`httpCreateTaskRequest`) |
| CLI | `kandev task create --require ci-at-head --require review-at-head --protect <glob> --max-rework <n> --max-attempts <n>` (`cmd/agentctl/kandev_task.go`), through `office/runtime` `CreateTaskInput` |
| MCP | `create_task_kandev` gains `completion_criteria` (`mcp/handlers/create_task_mode.go`, schema in `mcp/server/server.go`) |
| Routines | `taskTemplate` gains `completion_criteria` (`office/routines/service.go`), passed through `RoutineTaskOptions` |

All four reach `taskservice.Service.CreateTask`, which calls the registered
`CompletionCriteriaResolver` (`completiongates.Service.ResolveCreateCriteria`)
for Office requests once repositories and the Office workflow are resolved.
Refusals are typed `CompletionCriteriaRefusal` codes mapped to 400. Rules:

- Requested criteria omitted (nil): the workspace default applies.
- Explicit list from a person: used as given, including empty.
- Explicit list from an agent or routine: the union of the request and the
  workspace default, so an agent cannot remove a default
  (`AC-OFFICE-COMPLETION-GATES-001.2`).
- Repository, Office-task and review-stage checks run here. A requested
  criterion that fails them refuses creation; a default that fails them is
  dropped and logged. (`AC-OFFICE-COMPLETION-GATES-001.3` also asks for a
  task activity entry; that is not implemented yet because the task does not
  exist when the resolver runs.)
- Caps above the ceiling from an agent or routine are refused
  (`completion_cap_above_ceiling`). A person's value above the ceiling sets
  `caps_confirmed_by`.
- Protected paths are validated with `doublestar.ValidatePattern`.
- Owner is the creating user, agent profile, or routine.

Workspace settings live in a new Office table
`office_completion_gate_settings(workspace_id PK, default_criteria_json,
default_max_attempts, default_max_rework, ceiling_attempts, ceiling_rework,
updated_at)`, exposed as `GET/PUT
/api/v1/office/workspaces/:wsId/completion-gates/settings`. Absent rows mean
no default criteria, defaults 3, ceilings 10. The workspace deletion handler
removes the row (`office/repository/sqlite/workspace_deletion.go`), and the e2e
reset deletes it.

## CI verifier

`completiongates.CIVerifier` subscribes to `github.task_pr.updated`. The payload
is the full `TaskPR` row. For its task the verifier loads the gate; it returns
at once when there is no `ci_at_head` criterion. Evaluations are serialized per
task with a keyed mutex and coalesced: a newer event for the same task replaces
a queued one.

For each linked, non-detached PR it calls a new
`github.Service.ObserveChecksAtHead(ctx, taskPR) (CheckObservation, error)`.
That method uses the REST check-selection path (`getPRStatus` →
`selectCurrentPRChecks` → `computeOverallCheckStatus` and
`summarizeSelectedChecks`) and returns `{HeadSHA, State, Observed, Skipped,
FailingNames, ActiveWorkflowRuns}`. It bypasses the feedback TTL cache. If the
returned head differs from the row's head the observation is discarded; the
next update re-evaluates.

Aggregation across PRs, first match wins:

1. Any PR `failure` → `fail` / `checks_failed`.
2. Protected-path match not allowed for these heads → hold
   `protected_path_changed` (see below).
3. Any PR `pending` → `pending` (running checks decide nothing yet, so an
   incomplete file list waits instead of spending an attempt).
4. Changed-file list incomplete → `cannot_evaluate` /
   `changed_files_incomplete`.
5. Any PR with no counted check (`Observed == Skipped` or zero) and no active
   workflow run: `cannot_evaluate` / `checks_missing` once
   `result_first_seen_at` for this heads revision is at least 10 minutes old,
   else `pending`.
6. Otherwise `pass`, verifier `kandev_ci`, evidence summary listing check
   counts.

The verifier records the result with `RecordTaskCompletionEvaluation`. The
first `pending` for a new heads revision stamps `result_first_seen_at`.

A one-minute sweep re-evaluates gated tasks whose CI criterion was never
evaluated or is `awaiting_checks` with `result_first_seen_at` older than 10
minutes, and runs once at startup for every unmet CI criterion
(`AC-OFFICE-COMPLETION-GATES-002.7`). The repository selects only due rows
(`TaskCompletionSweepQuery`) and the sweep pages through them by task ID, 100
per page, so due rows are never hidden behind rows that are not due.

A failed workflow-run read treats the PR as possibly having an active workflow
(`pending`), never as `checks_missing`. A failed changed-file read records a
`cannot_evaluate` / `changed_files_incomplete` result bound to the heads.

`pr_merged_unverified`: the guard reports it from the row, and the evaluation
command turns a first `pass` into `pending` / `pr_merged_unverified` inside its
transaction when a linked row is merged. A pass already recorded at the same
heads before the merge is kept.

## Protected paths

`github.Service.ChangedFilesAtHead(ctx, taskPR) ([]PRFile, complete bool,
error)` lists files, then re-reads the PR head and returns `complete=false` if
the head moved. `PATClient.ListPRFiles` is changed to paginate with
`getPaginated`, like `ListPRCommits`; `GHClient` already uses `--paginate`. The
list is complete when its length equals the PR's `changed_files` and is below
3000. The mock client keeps its in-memory list and reports its length.

Matching uses `doublestar.Match` against `Filename` and, for renames,
`OldPath`. Matches feed the hold detail (at most 20 paths).

A protected hold sets `held_reason = protected_path_changed` without counting a
rework round. Both decisions are task controls that require a protected hold
whose `result_subject_revision` equals the live heads and the heads the person
saw; anything else returns 409. **Allow** (`allow_protected_paths`) records the
person and the heads in `protected_allowed_*`, clears the hold, and triggers a
re-evaluation. A new heads revision no longer matches the allowed revision, so
a protected change needs a new approval. **Send back**
(`send_back_protected_paths`) needs a nonblank reason, clears the hold,
records `result=fail`, `result_reason=sent_back` and the reason as the result
detail at the seen heads, and runs the rework path of
[Result routing](#result-routing). The CI verifier skips a criterion whose
`sent_back` result is pinned to the live heads, so the person's decision
stands until a new head arrives.

Human routes: `POST
/api/v1/office/tasks/:id/completion-gate/criteria/:criterionId/protected-paths`
with `{action: "allow" | "send_back", expected_subject_revision, reason}`.
A stale `expected_subject_revision` returns 409.

## Review at head

`workflow_step_decisions` gains `pr_heads TEXT NOT NULL DEFAULT ''`
(`workflow/repository/phase2_sqlite.go`). `RecordDecisionInput`
(`office/engine_dispatcher/dispatcher.go`) and `engine.DecisionInfo` carry it.
Two producers fill it for gated tasks with a `review_at_head` criterion:

- `DashboardService.RecordAgentDecision`
  (`office/dashboard/agent_decisions.go`), before building the input.
- The human path `recordTaskDecision` (`office/dashboard/decisions.go`),
  for any identity including the synthetic admin
  (`AC-OFFICE-COMPLETION-GATES-004.4`).

Both read the canonical heads string through a `TaskPRHeads(ctx, taskID)`
reader shared with the task primitive.

After any decision commits, `completiongates.ReviewVerifier` recomputes the
criterion from the step's current decisions: each seat's newest non-superseded
decision. An active `rejected` or `changes_requested` decision records `fail`
(reference: that decision), even over a human approval. Otherwise, if each
decision-required reviewer seat, or a person, has an `approved` decision whose
`pr_heads` equals the current heads, it records `pass` with verifier kind
`review_seats`, verifier ID the last approver, and reference listing decision
IDs. Otherwise it records nothing.

Decisions from the assignee are written as today (the workflow quorum is
unchanged) but are skipped by `OnDecision`, and the runtime decision response
gains `notice: "review_self_approval_ignored"`.

## Fences

| Path | Fence |
| --- | --- |
| HTTP, plugin, MCP evidence for host kinds | `task_completion_verifier_reserved`, independent of identity |
| CI result | Only `CIVerifier` calls `RecordTaskCompletionEvaluation` for `ci_at_head`; the command is not routed |
| Review result | Assignee decisions are ignored by `ReviewVerifier` |
| Criteria and caps edits | `CompletionGateCaller` from the run JWT; assignee refused |
| Cap extension by agents | Runtime route checks the caller is the owner agent or the workspace CEO, and not the assignee |

Residual risk, accepted: with authentication off the human decision endpoint
attributes every caller to the synthetic admin, so a local process that
calls it directly can approve as a person. CI criteria are not exposed to this
risk. The user is told at configuration time (see
[User interface](#user-interface)) and decides whether to enable
authentication; Kandev does not refuse the configuration.

## Result routing

`completiongates.Router` implements the verifiers' `EvaluationPolicy` and
subscribes to `task.moved`, `task.updated` and `github.task_pr.updated`.
Workflow-engine transitions (a worker's turn-complete handoff, reviewer quorum,
request changes) publish only `task.updated`; for an Office task whose
`step_transition_id` is new, the router reads that `task_step_transitions` row
for the source step. A per-task memory of the newest routed transition makes a
move seen through both events route once. Routing ignores only completed and
cancelled tasks; a failed run leaves an Office task routable. It moves tasks
through the task service's existing `MoveTask` with no human identity, so
moves pass normal admission and publish normal events. Every rework round is
counted by the task command `count_rework` with a key. The command keeps the
last 32 counted keys and reports whether this call counted, so redelivery never
counts twice. At the rework cap the same command holds the criterion and does
not keep the key, so the same return counts once the cap is raised.

- **CI `fail` recorded, task past the work stage:** count (key
  `ci:<heads>`) and move to the work stage. The work stage's `on_enter`
  `auto_start_agent` wakes the assignee.
- **Task leaves the work stage while CI fails at the live heads:** count
  (key `move:<step transition id>`) and move it back, so reviewers never
  review failing CI.
- **CI `fail`, task in the work stage:** no move and no count.
- **`cannot_evaluate`:** `Prepare` asks the evaluation command to count an
  attempt, or holds at the attempt cap.
- **Reviewer rejection:** a move from the review stage to the work stage on a
  task with a review criterion counts a rework round only when the newest
  decision at the review step is a rejection, keyed by that decision
  (`rejection:<decision id>`); the workflow already made the move. A manual
  move after an approval counts nothing.
- **Task leaves the review stage forward with the review criterion unmet:**
  the router first evaluates that step's decisions (the quorum transition
  commits before the decision hook records `pass`), then returns the task to
  the review stage, counted. Re-entering the review stage runs its `on_enter`
  (clear decisions, wake reviewers).
- **Heads move after review passed:** the router records `pending` /
  `head_moved` at the new heads (clearing the proof) and, when the task is past
  the review stage, returns it there, counted (key `head:<heads>`). A task in
  the review or work stage stays; approvals recorded against the old heads no
  longer count, and the forward-move rule above catches it when it leaves the
  review stage.
- **All criteria pass:** the router calls the engine's new
  `ReevaluateGuardedTransitions` (through
  `engine_dispatcher.Dispatcher.RetryGuardedTransitions`) for the current step
  once per heads revision (`AC-OFFICE-COMPLETION-GATES-006.3`), with a target
  filter that admits only the workflow's completing steps. Other satisfied
  guarded transitions are not re-applied. Unsatisfied guards do nothing.

Protected-path **Send back** counts a rework round (key `send-back:<heads>`).
A task past the work stage returns to it, and the work stage's entry wakes the
assignee. A task already in the work stage stays put, and the service queues a
`task_changes_requested` run for the assignee with the reason as
`decision_comment` (idempotency key `completion_gate_send_back:<task>:<key>`).

After a cap extension that releases a hold, the router re-runs the return the
hold stopped (a CI failure back to the work stage, or a review left stale by a
push back to the review stage) with the original key.

The Office task prompt builder (`office/service/prompt_builder.go`) adds a
**Completion criteria** section when a gated task has unmet criteria: each
criterion, its blocker, result, reason, and detail (failing checks or
protected files). `SchedulerIntegration.SetCompletionGateReader` supplies the
gate. This is how the woken worker learns why it is back.

## Caps, holds and escalation

`completiongates.EffectiveCaps(criterion, completingStep, settings)` resolves
each cap: criterion value, then the completing step's
`completion_gate_caps`, then the workspace default, then 3. It clamps to the
workspace ceiling unless `caps_confirmed_by` is set.

`completion_gate_caps` is a new optional workflow-step field
(`{max_verification_attempts, max_rework_rounds}`) on the step model, the
step DTO, portable export and import, and workflow sync, retaining explicit
values. Only the task's completing step's value is read.

While a criterion is held, `SchedulerService.ProcessRunGuard` skips runs
whose agent is the task's assignee and whose task is that task, with skip
reason `completion_gate_held`. Escalation runs are not affected.

Escalation follows the blocker-failure pattern
(`office/service/blocker_failure_wakeup.go`):

- Recipient: the owner agent when it exists and is active; else the first
  workspace CEO agent. A `user` owner, or a `routine` owner without an agent,
  gets an Inbox item instead.
- Wake: `agent_wakeup_requests` with idempotency key
  `completion_gate_held:<task>:<criterion>:<held_at RFC3339Nano>` and a
  prompt (`prompt_completion_gate_held.go`) carrying the criterion's result
  history from the gate history.
- Marker: a successful wake sets `escalated_at` through the
  `mark_escalated` control. A one-minute sweep retries every held,
  unescalated criterion, so a failed or impossible wake is retried.
- Inbox: derived projections in `office/dashboard/service_inbox.go` with kinds
  `completion_gate_protected_path` (always, for people) and
  `completion_gate_held` (person owners, and any cap hold not yet
  escalated). They are computed from held criteria, so releasing the hold
  removes them.

Cap extension: runtime route `POST
/api/v1/office/runtime/tasks/:id/completion-gate/extend` (CLI `kandev task gate
extend --criterion <id> [--attempts n] [--rework n]`), within the ceiling, and
human route `POST
/api/v1/office/tasks/:id/completion-gate/criteria/:criterionId/extend`, above
the ceiling allowed. Both apply the `extend_caps` control, which releases a
cap hold only when the cap that caused it rises above its used count.
`GET /api/v1/office/tasks/:id/completion-gate/caps` returns each host
criterion's effective caps and counts for the task page.

On the public set-criteria command the owner is always the acting caller and
only a person confirms caps above the ceiling, as themselves; caller-supplied
owner and confirmation fields are discarded. Workspace settings updates refuse
agent callers.

## User interface

The Office task page (`components/task/simple/OfficeSimplePane.tsx`) renders a
new `CompletionCriteriaSection` in the main column after `TaskActionRow`,
only when the task status summary reports `criteria_count > 0`. It reads `GET
/api/v1/tasks/:id/completion-gate` and the effective caps, and refetches when
the status summary's gate `revision` or `state_revision` changes. Every gate
write (criteria set, evidence, result, hold, control) bumps `state_revision` on
`task_completion_sets`, and the batch and standalone summaries both carry it.

The header shows the title only, plus **Done** or **Done by override · actor**
once the task is done. Each row shows the kind name (**CI**, **Review**), the
status badge, a detail line, and the rework and attempt counts against the
caps. The detail line (`lib/completion-gates/criterion-detail.ts`) names what
decided the result, then the short heads (7 characters per PR): failing check
names, changed protected files, or a send-back reason for CI; **Needs
approval**, or the reviewers who approved (decision IDs in
`evidence.reference`) or requested changes (the task's active
`changes_requested` decisions) for review. Each reviewer carries the agent
avatar or the person icon. Other states keep their reason text. Actions open
dialogs (bottom sheets on phones): **Override completion** (reason required;
calls the existing move with `TaskCompletionMoveOverrideRequest` to the
completing step), **Allow for this commit**, **Send back**, **Allow more
attempts**. Inbox items reuse the same dialogs.

The new-task dialog (`app/office/components/new-task-dialog.tsx`) gets a
**Completion criteria** group prefilled from workspace settings. Workspace
settings (`app/office/workspace/settings/components/settings-content.tsx`) get
a **Completion gates** card for defaults, caps and ceilings. API clients live
in `lib/api/domains/task-completion-gates-api.ts` and
`office-extended-api.ts`. All copy is in seven locales.

Both the new-task group and the settings card render an inline warning
(`AC-OFFICE-COMPLETION-GATES-008.8`) when **Review at PR head** is selected and
`state.auth.mode` is not `enabled`. It is a non-blocking alert with a link to
the Feature Toggles settings page; it reads only existing boot state, so no
backend change is needed.

The phone composition follows the existing Office task page: one scrolling
main column; the section uses full-width rows and 44-pixel action targets.

## Persistence and recovery

All state is in task and Office tables. On restart the CI sweep re-evaluates
unmet CI criteria; holds, counters and Inbox items survive because they are
rows. Every new SQL path gets SQLite and PostgreSQL replay tests and
store-conformance updates (`internal/persistence/storeconformance`).

## Observability

Expvar counters, closed label sets, no task or criterion IDs:

- `office_completion_gate_evaluation_total{kind, result}`
- `office_completion_gate_hold_total{reason}`
- `office_completion_gate_rework_total{kind}`
- `office_completion_gate_verifier_refused_total{reason}` (`verifier_reserved`,
  `self_verification`, `self_approval_ignored`)

Each is also logged as a structured zap entry.

## Related documents

- [Requirements](../requirements/completion-gates.md)
- [Task completion requirements](../../tasks/requirements/task-completion.md)
- [Host-verified completion criteria](../../tasks/requirements/task-completion-host-verified.md)
- [Coordination controls design](../../tasks/system-design/coordination-controls.md)
- [Blocker terminal policy design](blocker-terminal-policy.md)
- [GitHub PR check results](../../integrations/system-design/github-pr-check-results.md)
- [ADR: Host-verified completion criteria](../../../decisions/2026-11-20-host-verified-completion-criteria.md)
- [Implementation plan](../../../plans/office-completion-gates/plan.md)
