---
status: current
system: office
requirements:
  - REQ-OFFICE-COMPLETION-GATES-001
  - REQ-OFFICE-COMPLETION-GATES-002
  - REQ-OFFICE-COMPLETION-GATES-003
  - REQ-OFFICE-COMPLETION-GATES-004
  - REQ-OFFICE-COMPLETION-GATES-005
  - REQ-OFFICE-COMPLETION-GATES-006
  - REQ-OFFICE-COMPLETION-GATES-007
created: 2026-11-20
---

# Office Completion Gates System Design

## Requirement mapping

| Requirement | Design section |
| --- | --- |
| `REQ-OFFICE-COMPLETION-GATES-001` | [Creation surfaces and workspace defaults](#creation-surfaces-and-workspace-defaults) |
| `REQ-OFFICE-COMPLETION-GATES-002` | [Check verifier (github runner)](#check-verifier-github-runner) |
| `REQ-OFFICE-COMPLETION-GATES-003` | [Protected paths](#protected-paths) |
| `REQ-OFFICE-COMPLETION-GATES-004` | [Review at head](#review-at-head) |
| `REQ-OFFICE-COMPLETION-GATES-005` | [Fences](#fences) |
| `REQ-OFFICE-COMPLETION-GATES-006` | [Result routing](#result-routing) |
| `REQ-OFFICE-COMPLETION-GATES-007` | [Limits, holds and escalation](#limits-holds-and-escalation) |
| `REQ-OFFICE-COMPLETION-GATES-008` | [Part 1: Person actions and User interface](completion-gates-01.md) |
| `REQ-OFFICE-COMPLETION-GATES-009` | [Part 1: No reviewer available](completion-gates-01.md#no-reviewer-available) |

## Purpose and boundaries

The task system owns the gate primitive: criteria, evidence, results, holds,
counters, the completing-move guard, overrides and history. Its extension for
this feature is `REQ-TASKS-COMPLETION-006`, designed in
[Task-owned primitive](#task-owned-primitive) below and in the
[coordination controls design](../../tasks/system-design/coordination-controls.md#completion-gates).

Office owns policy: which tasks carry criteria, the check and review verifiers,
what a result does to the workflow, limit resolution, escalation, and the Office
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
| `kind` | `attested` (default), `check`, `review` |
| `config_json` | `{"runner": "github", "protected_paths": [...], "verification_limit": n, "rework_limit": n, "review_wait_limit": minutes, "limits_confirmed_by": "<user>"}`; `runner` is required on a check and absent elsewhere, `review_wait_limit` only on a review |
| `owner_kind`, `owner_id` | `user`, `agent`, `routine`, or `plugin`, and its ID |
| `result`, `result_reason`, `result_detail_json` | Latest evaluation (`AC-TASKS-COMPLETION-006.4`); detail is bounded (at most 20 names, 200 bytes each) |
| `result_subject_revision`, `result_first_seen_at`, `result_at` | The PR heads the result observed and when Kandev first saw them |
| `attempt_count`, `rework_count` | Counters (`AC-TASKS-COMPLETION-006.5`) |
| `hold_kind`, `held_at` | Hold (`AC-TASKS-COMPLETION-006.6`); empty when not held. Values: `protected_files_changed`, `rework_limit_reached`, `verification_limit_reached`, `no_reviewer_available`, `review_wait_limit_reached` |
| `protected_allowed_revision`, `protected_allowed_by` | Person approval of a protected-path change at one heads revision |

A host criterion's ID is derived from its kind: `<runner>_check` for a check
(`github_check`) and `review` for a review (`models.TaskCompletionCriterionID`).

`criterion_revision` increments when `kind`, `description`, or
`config_json.protected_paths` changes. That reset zeroes counters, clears the
hold and the protected approval, and sets `result` to `pending`
(`completionCriterionEquivalent` gains `kind` and `protected_paths`). A limit
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
- `ApplyTaskCompletionControl` applies one recovery action and appends
  history with the actor, the reason, the hold kind and the verdict source:
  `hold` (sets a `hold_kind` with its detail and the heads it holds at; a
  repeat of the same hold writes nothing), `release_hold` (clears only the named
  hold kind), `extend_limits`, `allow_protected_paths`, `send_back`,
  `accept`, `count_rework` and `mark_escalated`. `allow_protected_paths`,
  `send_back` and `accept` are bound to the heads the person saw and return
  409 when the live heads differ.
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

### Verdict source

The verdict source is derived, never stored
(`models.TaskCompletionVerdictSource`): empty while unverified; `person` when
the verifier kind is `human` (a person's acceptance or a person's attested
evidence); otherwise `program` for a check and `agent` for every other kind.
Each criterion in the snapshot carries `verdict_source`, and every evaluation
and control history entry records it in `details`. There is no task-level
summary: the snapshot and the status summary carry no weakest source, and the
status summary's gate projection keeps only the counts lists already used plus
`state_revision`.

The snapshot also carries `heads_revision`, the live PR heads of a task with
host criteria, so a person's action on a criterion that has no result yet is
pinned to the commit the page showed.

## Creation surfaces and workspace defaults

| Surface | Change |
| --- | --- |
| Office new-task dialog | `POST /api/v1/tasks` body gains `completion_criteria` (`httpCreateTaskRequest`) |
| CLI | `kandev task create --require github-check --require review --protect <glob> --rework-limit <n> --verification-limit <n> --review-wait-limit <duration>` (`cmd/agentctl/kandev_task_gate.go`), through `office/runtime` `CreateTaskInput` |
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
- Limits above the ceiling from an agent or routine are refused
  (`completion_limit_above_ceiling`). A person's value above the ceiling sets
  `limits_confirmed_by`.
- Protected paths are validated with `doublestar.ValidatePattern`.
- A check must name the `github` runner and carry no review wait limit; a
  review names no runner. The repository applies the same rule.
- Owner is the creating user, agent profile, or routine.

Edits go through the same policy: `completiongates.Service` also implements
`taskservice.CompletionCriteriaChangeValidator`, which
`SetTaskCompletionCriteria` and `SetTaskCompletionCriteriaExact` (the plugin
write path) call before writing. It refuses a review criterion when the task's
workflow has no review stage with the same
`completion_criteria_requires_review_stage` refusal, mapped to 400 by the task
HTTP handler. Plugin criteria cannot name a kind, so the plugin path never adds
a review; the check still runs.

Workspace settings live in a new Office table
`office_completion_gate_settings(workspace_id PK, default_criteria_json,
default_verification_limit, default_rework_limit, verification_limit_ceiling, rework_limit_ceiling,
default_review_wait_limit, updated_at)`, exposed as `GET/PUT
/api/v1/office/workspaces/:wsId/completion-gates/settings`. Absent rows mean
no default criteria, defaults 3, ceilings 10, and no review wait limit. The workspace deletion handler
removes the row (`office/repository/sqlite/workspace_deletion.go`), and the e2e
reset deletes it.

## Check verifier (github runner)

`completiongates.CIVerifier` subscribes to `github.task_pr.updated`. The payload
is the full `TaskPR` row. For its task the verifier loads the gate; it returns
at once when there is no `check` criterion. Evaluations are serialized per
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

1. Protected-path match not allowed for these heads → hold
   `protected_files_changed` (see below). It comes first so a failing check
   never sends a protected change back to the worker before a person sees it
   (AC-OFFICE-COMPLETION-GATES-003.1).
2. Any PR `failure` → `fail` / `checks_failed`.
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

A one-minute sweep re-evaluates gated tasks whose check was never
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

A protected hold sets `hold_kind = protected_files_changed` without counting a
rework round. Both decisions are task controls that require a protected hold
whose `result_subject_revision` equals the live heads and the heads the person
saw; anything else returns 409. **Allow for this commit** (`allow_protected_paths`) records the
person and the heads in `protected_allowed_*`, clears the hold, and triggers a
re-evaluation. A new heads revision no longer matches the allowed revision, so
a protected change needs a new approval. **Send back**
(`send_back`) needs a nonblank reason, clears the hold,
records `result=fail`, `result_reason=sent_back` and the reason as the result
detail at the seen heads, and runs the rework path of
[Result routing](#result-routing). The CI verifier skips a criterion whose
`sent_back` result is pinned to the live heads, so the person's decision
stands until a new head arrives. The repository enforces the same rule inside
the evaluation transaction: a verifier result at the heads of a person's
`sent_back` or `accepted` result changes nothing, even when the verifier read
its inputs before the person acted.

Human routes: see [Person actions](#person-actions).

## Review at head

`workflow_step_decisions` gains `pr_heads TEXT NOT NULL DEFAULT ''`
(`workflow/repository/phase2_sqlite.go`). `RecordDecisionInput`
(`office/engine_dispatcher/dispatcher.go`) and `engine.DecisionInfo` carry it.
Two producers fill it for gated tasks with a `review` criterion:

- `DashboardService.RecordAgentDecision`
  (`office/dashboard/agent_decisions.go`), before building the input.
- The human path `recordTaskDecision` (`office/dashboard/decisions.go`). A
  person's decision keeps its heads for the workflow's own approval quorum,
  but the review criterion ignores it (`AC-OFFICE-COMPLETION-GATES-004.2`).

Both read the canonical heads string through a `TaskPRHeads(ctx, taskID)`
reader shared with the task primitive.

After any decision commits, `completiongates.ReviewVerifier` recomputes the
criterion from the decisions of the step's required reviewer seats only (role
`reviewer`, `decision_required`): each seat's newest non-superseded decision.
Decisions under any other participant, including a person's, are ignored. An
active `rejected` or `changes_requested` seat decision records `fail`
(reference: that decision). Otherwise, if every required seat has an
`approved` decision whose `pr_heads` equals the current heads, it records
`pass` with verifier kind `review_seats`, verifier ID the last approver, and
reference listing decision IDs. Otherwise, and when the step has no required
seat, it records nothing. A person's `sent_back` result pinned to the live heads
is left alone, so approvals cannot override it before the heads move.

Decisions from the assignee are written as today (the workflow quorum is
unchanged) but are skipped by `OnDecision`, and the runtime decision response
gains `notice: "review_self_approval_ignored"`.

## Fences

| Path | Fence |
| --- | --- |
| HTTP, plugin, MCP evidence for host kinds | `task_completion_verifier_reserved`, independent of identity |
| CI result | Only `CIVerifier` calls `RecordTaskCompletionEvaluation` for `check`; the command is not routed |
| Review result | Assignee decisions are ignored by `ReviewVerifier` |
| Criteria and limits edits | `CompletionGateCaller` from the run JWT; assignee refused |
| Person actions | Refused for a request carrying an Office run token (agent claims or `CompletionGateCaller`), with authentication on or off |
| Limit extension by agents | Runtime route checks the caller is the owner agent or the workspace CEO, and not the assignee |

Residual risk, accepted: with authentication off every caller without a run
token is the synthetic admin, so an agent's shell calling the API on localhost
looks identical to the person and can take every person action (Allow, Send
back, Accept, Allow more rounds or attempts, Override). The run-token fence
cannot catch it. The person is warned where they act (see
[part 1](completion-gates-01.md#user-interface)) and decides whether to enable
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
counts twice. At the rework limit the same command holds the criterion and does
not keep the key, so the same return counts once the limit is raised.

- **CI `fail` recorded, task past the work stage:** count (key
  `ci:<heads>`) and move to the work stage. The work stage's `on_enter`
  `auto_start_agent` wakes the assignee.
- **Task leaves the work stage while CI fails at the live heads:** count
  (key `move:<step transition id>`) and move it back, so reviewers never
  review failing CI.
- **CI `fail`, task in the work stage:** no move and no count.
- **`cannot_evaluate`:** `Prepare` asks the evaluation command to count an
  attempt, or holds at the attempt limit.
- **Reviewer rejection:** a move from the review stage to the work stage on a
  task with a review criterion counts a rework round only when the newest
  decision at the review step is a rejection, keyed by that decision
  (`rejection:<decision id>`); the workflow already made the move. A manual
  move after an approval counts nothing. The work stage's `auto_start_agent`
  asks `Service.AssigneeStartHeld` before it launches the assignee, which
  counts the same keyed rejection first (`Router.SettleWorkEntry`), so a
  rejection at the rework limit holds the review before the worker starts.
  Whichever of the start check and the move event applies the hold escalates
  it: `count_rework` reports `claimed` for the call that applied the hold.
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
- **Heads move while the review is held:** the router keeps the hold, its
  kind and its cause, and moves the hold to the new heads (`hold` control with
  the new `SubjectRevision`, no new escalation), so **Accept** and **Send back**
  act on the commit the person is shown. The move carries `RebindFromSubject`:
  it applies only while the same hold still sits at the heads the router read,
  and only to the live heads, so a hold cleared meanwhile is not restored and
  overlapping pushes cannot bind it to older heads. A confirmation opened for the earlier
  heads is refused with `task_completion_evidence_changed`. A hold recorded
  before any PR was linked gets its heads the same way. A person's verdict
  (`sent_back`, `accepted`) belongs to the commit it was given at, so under a
  hold it caused (a send back at the rework limit) the router instead records
  `pending` / `head_moved` at the new heads and keeps the hold; raising the
  limit then routes the review normally and never replays the old send back.
- **All criteria pass:** the router calls the engine's new
  `ReevaluateGuardedTransitions` (through
  `engine_dispatcher.Dispatcher.RetryGuardedTransitions`) for the current step
  once per heads revision (`AC-OFFICE-COMPLETION-GATES-006.3`), with a target
  filter that admits only the workflow's completing steps. Other satisfied
  guarded transitions are not re-applied. Unsatisfied guards do nothing.

A person's **Send back** (a protected-files hold on a check, or **Send back at
this commit** on a review) counts a rework round (key `send-back:<heads>`),
exactly like a seat rejection.
A task past the work stage returns to it, and the work stage's entry wakes the
assignee. A task already in the work stage stays put, and the service queues a
`task_changes_requested` run for the assignee with the reason as
`decision_comment` (idempotency key `completion_gate_send_back:<task>:<key>`).

After **Accept at this commit**, when the gate is no longer blocked, the
router runs the same guarded-transition retry as an all-pass result; Accept
itself never moves the task.

After a limit extension that releases a hold, the router re-runs the return the
hold stopped (a CI failure back to the work stage, or a review left stale by a
push back to the review stage) with the original key.

The Office task prompt builder (`office/service/prompt_builder.go`) adds a
**Completion criteria** section when a gated task has unmet criteria: each
criterion, its blocker, result, reason, and detail (failing checks or
protected files). `SchedulerIntegration.SetCompletionGateReader` supplies the
gate. This is how the woken worker learns why it is back.

## Limits, holds and escalation

`completiongates.EffectiveLimits(criterion, completingStep, settings)` resolves
each limit: criterion value, then the completing step's
`completion_gate_limits`, then the workspace default, then 3. It clamps to the
workspace ceiling unless `limits_confirmed_by` is set.

The review wait limit resolves from the criterion, then the completing
step's `completion_gate_limits.review_wait_limit`, then the workspace
`default_review_wait_limit`, in minutes. Zero everywhere means a parked review
waits indefinitely; it has no ceiling because waiting starts no runs.

`completion_gate_limits` is a new optional workflow-step field
(`{verification_limit, rework_limit, review_wait_limit}`) on the step model, the
step DTO, portable export and import, and workflow sync, retaining explicit
values. Only the task's completing step's value is read.

While a criterion is held, `SchedulerService.ProcessRunGuard` skips runs
whose agent is the task's assignee and whose task is that task, with skip
reason `completion_gate_held`, and the orchestrator's `auto_start_agent`
callback skips a launch whose session belongs to the assignee
(`Service.SetAssigneeStartHold`). Escalation runs are not affected.

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
- Prompt: the escalation names the hold kind and what the owner can do: raise
  a limit with `kandev task gate extend` for a limit hold, or add a reviewer
  agent on another provider, reassign, or ask a person to accept the review
  for `no_reviewer_available` and `review_wait_limit_reached`.
- Inbox: derived projections in `office/dashboard/service_inbox.go` with the
  single kind `completion_gate_hold`; the payload carries the hold kind, the
  detail, the heads and the counts. A protected-files hold always shows; other
  holds show for person owners and while not escalated to an agent. They are
  computed from held criteria, so releasing the hold removes them.

Limit extension: runtime route `POST
/api/v1/office/runtime/tasks/:id/completion-gate/extend` (CLI `kandev task gate
extend --criterion <id> [--verification-limit n] [--rework-limit n]`), within the ceiling, and
human route `POST
/api/v1/office/tasks/:id/completion-gate/criteria/:criterionId/extend`, above
the ceiling allowed. Both apply the `extend_limits` control, which releases a
limit hold only when the limit that caused it rises above its used count.
When the released criterion holds a person's `sent_back` result, the service
resumes that send back: it counts the round under the original
`send-back:<heads>` key, returns the task to work, and wakes the worker with
the person's reason when the task is already there. Other released failures
route through `Router.RouteCriterion`.
`GET /api/v1/office/tasks/:id/completion-gate/limits` returns each host
criterion's effective limits and counts for the task page.

On the public set-criteria command the owner is always the acting caller and
only a person confirms limits above the ceiling, as themselves; caller-supplied
owner and confirmation fields are discarded. Workspace settings updates refuse
agent callers.

## People, status and reviewer availability

How a review that no agent can approve holds, the person actions, the user
interface, and the phase 2 check runner design are in
[part 1](completion-gates-01.md).

## Persistence and recovery

All state is in task and Office tables. On restart the CI sweep re-evaluates
unmet checks; holds, counters and Inbox items survive because they are
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
- [Part 1: people, status and reviewer availability](completion-gates-01.md)
- [Task completion requirements](../../tasks/requirements/task-completion.md)
- [Host-verified completion criteria](../../tasks/requirements/task-completion-host-verified.md)
- [Coordination controls design](../../tasks/system-design/coordination-controls.md)
- [Blocker terminal policy design](blocker-terminal-policy.md)
- [GitHub PR check results](../../integrations/system-design/github-pr-check-results.md)
- [ADR: Host-verified completion criteria (superseded)](../../../decisions/2026-11-20-host-verified-completion-criteria.md)
- [ADR: People act on exceptions](../../../decisions/2026-11-21-completion-gates-people-act-on-exceptions.md)
- [Implementation plan](../../../plans/office-completion-gates/plan.md)
