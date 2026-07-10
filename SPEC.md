# Specification

> Solution-space document for GitHub issue
> [#1597](https://github.com/kdlbs/kandev/issues/1597) — "Executor-row
> desync persists on v0.73.0". Derived from
> [REQUIREMENTS.md](./REQUIREMENTS.md). Each section cites the
> `§req:` it satisfies.
>
> **One theme, two layers.** The unifying defect is *the session lies
> about being busy*. It manifests in the durable `executors_running`
> table (rows claim a process that is gone) and in the live prompt
> path (a session that looks busy against a turn/process that is
> actually finished or dead). ADR 0025 promoted `executors_running` to
> the authoritative runtime inventory without giving its liveness
> columns a trustworthy producer; this spec makes the table truthful
> rather than reverting that decision.
>
> **Scope note (2026-07-06).** The live-path half was root-caused and
> fixed upstream while this work was in review: #1600 (Claude Monitor
> callbacks emitted content outside a prompt RPC, so the turn never
> completed and the session stayed "busy" forever) and #1602 (resume
> reused executions whose ACP prompt path was dead even though the
> process looked alive). This spec therefore owns the durable-table
> half plus pause→resume consistency. A finer-grained busy signal
> (accepting input while the foreground turn is idle on background
> work, and mid-turn steering for agents that support it) is a
> follow-up product feature tracked separately; a working prototype of
> the background-idle half lives on branch
> `archive/1597-full-six-batches`.
>
> **Scope note (2026-07-10).** Those two follow-ups are now specified
> below and their feasibility settled by discovery. The background-idle
> half (§spec:fine-grained-busy-signal) is buildable in Kandev alone and
> is the target of the current run. Mid-turn steering
> (§spec:mid-turn-steering) is **not** buildable in Kandev alone: the ACP
> protocol has no verb to deliver input into an open turn, and the Claude
> Agent SDK queues mid-turn input until the turn ends (its only mid-turn
> control is a hard interrupt) — the capability is gated on the
> upstream ACP v2 `session/inject` proposal
> ([agent-client-protocol #1220](https://github.com/orgs/agentclientprotocol/discussions/1220),
> drafted as PR #1261). That section is recorded as a gated end-state, not
> as buildable-now work. Note: `REQUIREMENTS.md` was re-scoped to mid-turn
> steering (#1607) after the executor-row sections above were written, so
> those sections' `§req` numbers refer to the pre-pivot requirements;
> the two sections below cite the current file.

## Pause then resume §spec:pause-resume-recovery

*Status: done — pausing a running turn settles the session to
WAITING_FOR_INPUT and `Service.CancelAgent` now drains the message queue
directly after reconciling, so a queued message is delivered on resume
even on the escalated / dead-process cancel path where no `agent.ready`
event fires (idempotent with the event-driven drain). The resume path
distinguishes a genuinely-generating foreground turn (reject) from a row
that looks live but whose process is gone (clean + relaunch via
`resume_token`), reusing the runtime-aware liveness from
§spec:truthful-executor-rows. Reachable from the pause button via the
`agent.cancel` WS action. Corrected contracts:
`TestCancelAgent_DeliversQueuedMessageOnResume` (replaces the wedge test
`TestCancelAgent_LeavesQueuedMessageForManualDrain`) and
`TestResumeSession_StaleExecutionCleansUpAndRetries` (now asserts the
relaunch reuses the row's `resume_token`); Playwright coverage in
`pause-resume-recovery.spec.ts`.*

**Problem.** On v0.73.0 an operator who pauses a running agent turn
cannot resume it: the next message is dropped or rejected because the
session still looks "busy"/running, and the only recovery is to cancel
and restart the whole headless service — which every other session
depends on.

**Behavior.** Pausing is a first-class, reversible step in the
workflow:

- When the operator stops/pauses a running agent turn, the session
  settles into a state that accepts input (`WAITING_FOR_INPUT`) and its
  `executors_running` row is left resumable — the process is not
  orphaned and the row is not deleted while the session is still open.
- When the operator then sends a new message, the system resumes the
  same session with its context intact, using the row's `resume_token`,
  without a service restart.
- When a paused session's agent process is actually gone (crash, kill,
  escalated cancel), the resume path detects the dead process, cleans
  the stale execution, and relaunches — rather than reporting "already
  running" against a process that no longer exists.
- The queued-message contract is corrected: a message sent to a paused
  session is delivered on resume rather than being stranded for
  indefinite manual drain.

**Corrected test contracts.** Two existing tests currently codify the
wedge as expected behavior and are rewritten to the corrected
contract, red before green:

- `TestResumeSession_LiveAgentReturnsAlreadyRunning` — a session that
  merely *looks* live must not permanently reject a resume; the
  contract distinguishes "a foreground turn is genuinely generating"
  (reject/queue) from "the row looks live but the process is gone"
  (clean and relaunch). It no longer requires a live-*looking* session
  to reject a new message unconditionally.
- `TestCancelAgent_LeavesQueuedMessageForManualDrain` — a paused
  session's queued message is resumable, not stranded; the test
  asserts the message is delivered on resume.

**Why.** The pause→resume wedge is the operator's headline pain
(§req:problem-statement). The single-scalar `RUNNING` gate in
`checkSessionPromptable` rejects input for any session whose DB state
reads `RUNNING`, even after its process has died, and cancel does not
reconcile the row's liveness — so a routine pause silently wedges the
session. The corrected contract makes "busy" mean "a foreground turn is
actually generating," which is the precondition every other section in
this spec establishes.

**Alternatives rejected.** Re-landing #1594 (re-adding the `ready`
promotion) — rejected: the `ready` status already shipped in v0.73.0
via #1587 and is ineffective for the headless/standalone case, so it is
not the fix (§req:problem-statement note). Requiring the operator to
cancel-then-restart — rejected: it is the very symptom being removed.

**Tradeoffs.** Accepting input against a session that is technically
still finishing an escalated cancel requires the resume path to probe
real liveness (see §spec:truthful-executor-rows), adding a liveness
check on the hot resume path in exchange for correctness.

Satisfies §req:success-criteria #1, #4, #7, #8; §req:user-stories
(operator who paused to redirect); §req:priorities must-have #1, #2.

## Truthful executor rows §spec:truthful-executor-rows

*Status: done — local/standalone rows carry a real host-local liveness handle
(`executors_running.local_pid`), populated event-driven on every lifecycle
transition (launch, boot-ready, turn-complete, cancel, process-exit/crash via
MarkCompleted), with `status`, `agentctl_port`, and an observed `last_seen_at`.
Delivered by the foundation batch (#1597).*

**Problem.** For local/standalone (`local_pc`) runtimes every
`executors_running` row reports `pid = 0` and a `last_seen_at` that
only reflects the last lifecycle write, never a real liveness check.
Rows sit stuck at `starting`/`prepared` with no endpoint. Because the
row cannot distinguish a live session from a dead one, everything that
reads it — cancel cleanup, reconciliation, self-heal — makes wrong
decisions.

**Behavior.** A row describes the real process it mirrors, for the
local/standalone runtime and not only for SSH:

- When a session has finished launching and is between turns, its row
  reads `ready` (not stuck `starting`/`prepared`) and carries a real,
  currently-alive local liveness handle and endpoint — a live
  OS-process reference for the process Kandev spawned, plus
  `agentctl_port`, `agentctl_url`, and a `last_seen_at` that reflects an
  actual liveness observation — that an operator can cross-check against
  the host.
- Every lifecycle transition leaves the row correct: launch,
  boot-ready, turn-complete, cancel (including escalated cancel),
  process-exit / crash, and stop each update the row's `status`,
  endpoint, and liveness handle. Event hooks are the primary producer;
  no lifecycle transition leaves the row claiming a process that has
  exited.
- A dead row is distinguishable from a live one from the row alone
  (status plus a liveness handle whose process can be probed), so
  automated repair/prune can decide safely without external context.

**Why.** ADR 0025 made this table load-bearing but v0.66 never gave its
liveness columns a producer for local runtimes, so pre-existing drift
gained consequences (§req:problem-statement). The remedy is
event-driven correctness first: the row is made truthful by hooking
every transition, because polling can never be as timely or as cheap as
the event that already fires (§req:quality-attributes, event-driven
correctness first). The liveness handle is a real, currently-alive
process reference sufficient to detect dead rows — the process Kandev
already spawns is acceptable; it is not a new per-session runtime handle
(§req:constraints, PID scope).

**Alternatives rejected.** Reusing the SSH `pid` column for a local
process id — rejected: `executors_running.pid` holds the agentctl PID
*on the remote host* for SSH rows, and overloading it would silently
change that column's meaning and invite local-process checks against
remote rows (§req:constraints; see §spec:runtime-aware-liveness). A
periodic poll as the primary liveness producer — rejected: events are
primary, polling is a redundant backstop only (§spec:reconciliation-backstop).

**Tradeoffs.** Recording and probing a local process handle adds a
column and a liveness call at each hooked transition; accepted because
untrustworthy rows are the root cause of every downstream wrong
decision.

Satisfies §req:success-criteria #2, #3; §req:user-stories (operator
running headless for days); §req:priorities must-have #3.

## Runtime-aware liveness §spec:runtime-aware-liveness

*Status: done — `RowProcessLiveness` judges a row by its `runtime`: local rows
are probed by `local_pid`; SSH/remote and docker rows return Unknown so a
host-local process check never runs against a remote pid. The SSH remote-pid
stop path (`kill -0` over SSH) is unchanged. Delivered by the foundation batch
(#1597).*

**Problem.** Liveness semantics differ by runtime: an SSH row's process
id lives on a remote host, while a local row's process lives on the
Kandev host. A single local-process liveness check applied blindly
would either corrupt SSH rows or regress the SSH remote-pid stop path.

**Behavior.** Liveness and prune logic branch on the row's `runtime`:

- Local/standalone rows are judged by the local liveness handle
  (§spec:truthful-executor-rows); local-process existence checks never
  run against SSH (remote) rows.
- SSH rows retain their existing meaning: `pid` is the agentctl PID on
  the remote host, and the remote-pid stop path continues to stop the
  remote process over SSH unchanged.
- Reconciliation and pruning evaluate each row against the correct
  host for its runtime, so a remote row is never pruned because a local
  process check failed and vice versa.

**Why.** The operator explicitly flagged, and the code confirms, that
`pid` is SSH-only remote semantics; local liveness must not overload it
(§req:constraints, do not overload SSH pid). No-regression of the SSH
executor's remote-pid stop path is a stated quality attribute
(§req:quality-attributes).

**Alternatives rejected.** A single runtime-agnostic liveness predicate
— rejected because it cannot be correct for both a local process and a
remote pid; runtime-awareness is inherent to the problem.

**Tradeoffs.** Per-runtime branching in the liveness/prune paths adds
conditional complexity; accepted as the minimum required to avoid
cross-runtime corruption.

Satisfies §req:success-criteria #2, #3; §req:quality-attributes
(runtime-aware liveness, no regression); §req:constraints (SSH pid).

## Resume-safety invariant §spec:resume-safety-invariant

*Status: done (#1597 Batch 4)*

**Problem.** Losing the ability to resume a session is unacceptable,
yet multiple cleanup paths (cancel cleanup, startup reconciliation,
on-demand stale cleanup, task teardown) delete `executors_running`
rows, and any of them could delete a row that still backs an open,
resumable session.

**Behavior.** One ironclad rule governs deletion, enforced across every
cleanup path:

- A row backing a non-terminal session, or holding a `resume_token`, is
  repaired in place and never deleted. Only rows confirmed
  terminal/dead may be pruned.
- A session waiting for input before a restart is still resumable with
  full context afterward; no cleanup — on cancel, on startup, or
  otherwise — removes a row it still needs to resume.
- `resume_token` remains a single source of truth in
  `executors_running`; it is not duplicated into a second table.

**Why.** Data safety with a single source of truth is a hard
requirement (§req:quality-attributes). Duplicating `resume_token` into
a second table would reintroduce exactly the divergence risk this effort
removes, so the guarantee is expressed as a deletion invariant rather
than as redundant storage (§req:constraints, do not duplicate
resume_token).

**Alternatives rejected.** Copying `resume_token` into `task_sessions`
so a deleted runtime row is survivable — rejected: two writers of the
same fact is the divergence pattern being eliminated. Deleting rows
eagerly and relying on relaunch — rejected: it can destroy the only
handle to a resumable conversation.

**Tradeoffs.** Cleanup paths must classify a row as terminal/dead
before deleting, adding a check to each path; accepted because an
erroneous delete costs the operator a conversation.

Satisfies §req:success-criteria #7; §req:quality-attributes (data
safety); §req:constraints (do not duplicate resume_token);
§req:user-stories (operator with sessions waiting for input).

## Startup reconciliation §spec:reconciliation-backstop

*Status: done — startup reconciliation repairs live-looking rows whose
local process is confirmed dead (status=stopped, `local_pid` cleared,
`resume_token`/worktree preserved) and prunes only rows that fail
`RowMustBePreserved`. Scoped deliberately to startup: events (the
lifecycle hooks of §spec:truthful-executor-rows) are the primary
producer, and every in-app transition — including process exit/crash,
which the lifecycle manager observes — fires one. A periodic polling
backstop was built, then removed before merge: it defended against
failure modes (OOM kill, dropped event) that have not been observed,
and the live-wedge causes it would have self-healed were fixed at their
root upstream (#1600 completes idle async ACP turns; #1602 reaps
prompt-dead executions on resume).*

**Problem.** Nothing reconciled rows against reality at startup:
the pass preserved a row's `status` without re-checking the process,
and stale rows were pruned only on-demand per session. So after heavy
headless use the backlog of dead rows grew across restarts (~50 → 77,
measured in #1597).

**Behavior.** The startup pass makes rows true:

- On startup the system verifies rows against reality using
  runtime-aware liveness: rows whose local process is confirmed dead
  are repaired so they no longer claim a live process, and only rows
  whose session is confirmed terminal/dead are pruned, subject to the
  §spec:resume-safety-invariant.
- Reconciliation never deletes a row backing a non-terminal session or
  holding a `resume_token`.
- After a clean restart following heavy use, the count of stale rows
  (rows whose process is dead) trends toward zero rather than
  accumulating across restarts.

**Why.** Events are the primary producer; startup reconciliation exists
because a backend restart is exactly the moment events could not have
fired for whatever the previous process was doing when it died
(§req:quality-attributes, event-driven correctness first). ADR 0025
stays: the table remains the authoritative inventory and is made
trustworthy rather than reverted (§req:constraints, ADR 0025 stays).

**Alternatives rejected.** Polling as the primary means of updating the
table — rejected: events are primary and must be hooked wherever they
exist; polling never updates a row the moment an event could
(§req:quality-attributes). A periodic in-run backstop pass — built and
then removed: insurance for undemonstrated failure modes; can be
restored from branch `archive/1597-full-six-batches` if a stale row is
ever observed to appear *between* restarts. Reverting ADR 0025 to drive
cleanup off `task_sessions` again — rejected: it reintroduces multiple
paths deciding liveness, which ADR 0025 consolidated for good reason.

Satisfies §req:success-criteria #5, #6; §req:quality-attributes
(bounded growth, truthfulness/durability); §req:priorities must-have
#5; §req:user-stories (operator on a long-lived server).

## Fine-grained foreground-idle busy signal §spec:fine-grained-busy-signal

*Status: in progress — the backend behavior half (the foreground-idle busy
gate on the prompt path) has landed; the operator-visible composer/status
surfacing of the three-way signal is the remaining frontend half.*

**Behavior.** A session whose durable state reads RUNNING accepts a new
operator message whenever its foreground turn is *idle* — that is, the
foreground agent is not itself generating and the turn is only held open
by registered background work the agent spawned (a subagent task, a
run-in-background shell, or an active Monitor watch). When the foreground
agent is actively generating, or when no recognized background work is
outstanding, the session keeps rejecting input as busy exactly as today.
When the background work completes and the turn is once again
foreground-driven, input is gated again until the turn ends. The signal
is per-agent: only agents whose background shapes are recognized get the
narrowed gate; any agent whose in-flight frames are not recognized as
background (codex, opencode, gemini, and any future agent) defaults to
busy, byte-for-byte unchanged.

This corrects *whether* input is accepted; it does **not** deliver into a
live turn. An accepted message travels the normal send path: it reaches
the agent promptly when the foreground prompt exchange has already
returned (e.g. an out-of-turn Monitor burst or a backgrounded shell after
the main turn yielded), or at turn-end when a still-open subagent holds
the exchange open — the same delivery point as today's queue, but without
the false "agent is already running" rejection. Mid-turn delivery is the
separate, gated concern of §spec:mid-turn-steering.

**Background-in-progress indicator.** Accepting input while background
work runs must not make the session look *finished*. The session surfaces
three distinguishable conditions, not two: (a) the foreground agent is
actively generating, (b) the foreground turn is idle but spawned
background work is still outstanding, and (c) the session is fully idle
with nothing outstanding. Both (a) and (b) show a "working" status
affordance — the operator can see the agent is not done — but only in (b)
does the composer accept input; (c) shows the completed/idle affordance. A
session waiting only on background work therefore holds the in-progress
indicator (e.g. the spinning "working" state, not the done/"complete"
checkmark) even though the prompt box is enabled, and flips to the
completed affordance only once the last background task finishes. The
driving constraint: without this, a background-idle session is visually
indistinguishable from one that has fully finished, so the operator cannot
tell whether the agent is still doing anything. The truthful signal must
communicate two independent facts — "you may type" *and* "work is still in
progress (in the background)" — rather than collapsing them into a single
busy/done bit.

**User-level test path.** Drive a Claude session; have it spawn a
subagent or a `run_in_background` shell so the foreground turn is held
open but idle, or drive a chatty Monitor after the turn returned. Send
"are you still working?": the session accepts it instead of rejecting it
as busy, and the operator can see (in the composer) that the message was
accepted rather than silently diverted to the queue. Meanwhile the task
status indicator stays in its "working" state (the spinner, not the
done/complete checkmark) for as long as the background task runs, and
flips to complete only after it finishes. Repeat with a non-Claude agent
whose frames aren't recognized as background: input stays gated, no
change. Because both the accepted-vs-busy state and the
working-vs-complete state must be visible to be actionable, the composer
and the status indicator reflect the fine-grained signal (the reviewed
prototype is backend-only — the composer still derives "busy" from
`state === RUNNING`, so the frontend surfacing of both signals is part of
this capability, not a separate follow-up).

**Why.** The single durable session state (RUNNING vs WAITING_FOR_INPUT)
is too coarse to tell "the foreground agent is generating" from "the
foreground turn is idle, waiting on spawned background work." That
coarseness is what locks an operator out of the conversation for the
entire duration of a long background job. The driving constraint is that
this window is **not** closed by the upstream idle-turn completion
(#1600): #1600 only emits a synthetic turn-complete after async content
has been idle for a debounce interval, and it never arms while the
foreground prompt exchange is still in flight — so a genuinely held-open
turn (the agent is blocked awaiting a subagent) stays "busy" the whole
time, and chained event bursts keep re-extending #1600's debounce. The
system therefore tracks, per session and in memory, whether the open turn
is foreground-driven or has yielded to outstanding background work, and
narrows the busy gate to the foreground only. Default is busy:
unrecognized work and unrecognized agents preserve the exact historical
reject-while-RUNNING contract, so nothing regresses.

**Alternatives rejected.** *Rely on #1600's synthetic idle-turn
completion alone* — rejected by discovery: falsifiable checking of the
event flow shows #1600 cannot arm while the foreground prompt exchange is
open, so it structurally cannot cover a held-open turn, and its debounce
re-extends under bursts; the residual window is real. *Drop the RUNNING
gate / always accept input* — rejected: it would let a new message race a
genuinely-generating foreground turn, risking dropped or reordered
messages and regressing non-steering agents, violating
§req:quality-attributes (reliability and ordering; no regression).
*Persist the foreground/background distinction* — rejected as
unnecessary: the distinction only matters for a live in-flight turn, and
a backend restart ends the turn, so in-memory tracking that resets to the
safe "foreground generating" default on every turn-close is sufficient
and simpler. *Recognize background work by tool name string-matching* —
rejected: brittle across agents and updates; recognition keys on the
normalized shape of the work (subagent task / background shell / active
Monitor) so the producer and consumer share one contract.

**Tradeoffs.** The signal is best-effort across a backend restart — a
restart ends the turn anyway, so the gate resets to the safe default;
correctness is never traded, only the optimization. Recognition is
deliberately Claude-shaped today (subagent task, run-in-background shell,
active Monitor are Claude features); other agents simply keep today's
behavior, which is the intended conservative default rather than a gap.
Accepting input earlier does not make delivery earlier for the held-open
subagent case (that message still waits behind the open exchange, as the
queue does today); the win is a truthful signal — no false lockout, no
error, and prompt delivery for the out-of-turn cases — not mid-turn
influence. Surfacing "working in the background" as a status distinct from
both "generating" and "done" is new frontend work beyond the reviewed
backend prototype, but it is what makes the truthful signal legible to the
operator rather than a silent backend nicety.

**Requirements provenance.** The committed `REQUIREMENTS.md` (re-scoped to
mid-turn steering, #1607) currently *defers* this behavior, asserting it
"functions on current `main` via upstream #1600 / #1602"
(§req:constraints, §req:quality-attributes "no regression"). This spec's
discovery contradicts that assertion for the held-open and out-of-turn
windows, so this section realizes the intent captured **before** the
pivot — pre-pivot §req:success-criteria #8, "the session accepts input
whenever the foreground turn is idle … only *waiting on a spawned
background task*." A Discover pass should restore an explicit scope-A
success criterion so this citation is not dangling. Also satisfies
§req:quality-attributes (reliability and ordering; capability-gated, not
assumed) and §req:user-stories (the operator whose agent kicked off
background work wants to keep talking to the session).

## Mid-turn steering §spec:mid-turn-steering

*Status: gated — the end-state is specified, but live mid-turn delivery
is not buildable in Kandev alone. It depends on the upstream ACP v2
`session/inject` proposal
([agent-client-protocol #1220](https://github.com/orgs/agentclientprotocol/discussions/1220),
maintainer-endorsed, drafted as PR #1261). Recorded here so the intent is
durable and correctly gated; this run builds none of it.*

**Behavior (end-state).** While a capable agent is actively generating a
turn, an operator can type a message that reaches the agent *within that
same turn* — delivered at the agent's next safe break-point rather than
after the turn fully ends — so the operator can nudge a turn back on
course without cancelling it and discarding its in-progress work. The
message is editable and retractable until the moment the agent consumes
it, after which it is locked; a message the turn never consumes is
delivered seamlessly as the operator's next prompt and is never lost.
Steering is offered only where the agent genuinely supports it; every
other agent keeps today's queue-until-halt behavior with no new behavior
and no risk of dropped or reordered messages. The composer makes clear,
for the current agent and turn, whether a typed message will steer the
live turn or wait in the queue.

**User-level test path.** With a steering-capable agent mid-turn, type a
correction; observe the agent take it up inside the ongoing turn (no
cancel, no waiting for the turn to end). Edit the message before the
agent consumes it and confirm the agent receives the latest edit; retract
it before consumption and confirm nothing is delivered. Let a turn end
before consumption and confirm the message is delivered as the next
prompt. Repeat with a non-capable agent and confirm identical-to-today
queue behavior. (This path is exercisable only once the upstream gate
below is satisfied.)

**Why gated.** Discovery established that this cannot be faked in Kandev's
transport, and that the requirements' premise ("Claude natively supports
this") does not hold at the protocol layer:

- The ACP protocol Kandev speaks has exactly two turn verbs —
  `session/prompt` (a full request/response turn, serialized one-at-a-time
  so a second prompt blocks until the first ends) and `session/cancel`
  (which *ends* the turn). There is no verb to inject input into an open
  turn; sending two concurrent prompts corrupts the agent bridge's turn
  accounting. So the transport is architecturally one-turn-at-a-time.
- The programmatic path Kandev uses does not carry Claude's steering.
  Claude Code's CLI/TUI genuinely steers — its own input loop injects a
  mid-turn message into the running turn — but Kandev does not drive that
  TUI; it drives the Claude Agent SDK through the `claude-agent-acp`
  bridge (ACP over stdio, `stream-json` underneath). On that surface — as
  verified against the Claude Agent SDK docs — a message sent mid-turn is
  queued and delivered only *after* the current turn ends (streamed input
  is processed sequentially); the sole mid-turn control is a hard
  `interrupt()` that stops the turn, discarding its in-progress work,
  after which you re-prompt. Both outcomes — wait for the turn, or stop it
  — are exactly what §req:success-criteria #1 rules out. This is a *path*
  gap, not a model limitation: Anthropic's own open requests ask for "CLI
  steering parity" on the desktop app and SDK (claude-code #71726, #64624).
  So Claude B is reachable in principle, once Anthropic exposes a
  non-interrupting steer primitive on the SDK **and** ACP carries it.
- The upstream ACP v2 `session/inject` proposal (#1220) is the ACP half of
  that: agents advertise `session.inject.modes`, where **`queue` mode**
  (deliver at idle, FIFO) suits an agent whose programmatic surface has no
  safe mid-turn break-point, and **`steer` mode** (deliver at the next
  safe break-point) suits agents that expose one — such as Codex, whose
  app-server protocol already has a first-class non-interrupting
  `turn/steer` RPC.

The consequence for scope: **today**, only Codex exposes steering to a
programmatic client, so it is the nearer-term `steer`-mode target. On the
current SDK/ACP path **Claude is limited to `queue` mode** — deliver at
turn idle, which Kandev's existing editable/retractable message queue
*already does* — until Anthropic surfaces its (TUI-proven) steering on the
SDK, at which point Claude gains `steer` mode too. Capability *gating* is
cheap to add (the adapter already knows the concrete agent id and stores
the agent's advertised capabilities); the underlying *capability* on the
programmatic path is what does not yet exist.

**What already ships (so the buildable-now delta is small).** Requirements
§req:success-criteria #2 (editable), #3 (retract), #4 (never-lost), #5
(non-steering agents unchanged) and most of #6/#7 (affordance) are
**already satisfied today**: Kandev persists a per-session FIFO message
queue with edit, retract-one, and cancel-all, drains one entry per turn
at turn-end, leaves non-steering agents untouched, and shows a
queue affordance in the composer. The only unmet criterion is #1
(delivery *during* the turn) — the upstream-gated part.

**Alternatives rejected.** *Interrupt-and-resend (cancel the turn, then
re-prompt with the steer appended)* — rejected: it cancels the turn and
discards in-progress work, which §req:problem-statement and
§req:success-criteria #1 explicitly reject; it is a different, lossy UX,
not steering. *Bypass `promptGate` and send a concurrent `session/prompt`*
— rejected: it corrupts the agent bridge's turn accounting (stop-reasons
attach to the wrong turn), a correctness regression. *Open a new upstream
issue* — rejected: the exact proposal already exists (#1220), is
maintainer-endorsed, and is drafted as PR #1261; duplicating it adds
noise. The chosen path is to gate on #1220 and add Kandev's use-case to
that discussion.

**Tradeoffs.** Recording the end-state without building it risks
staleness if the upstream proposal changes shape; the section pins the
specific proposal (#1220 / PR #1261) and the capability-advertisement
contract (`session.inject.modes`) so drift is detectable. Framing Claude
as `queue`-only *on today's SDK/ACP path* is a correction to the
requirements' premise, not a reduction of ambition: Claude Code's TUI does
steer, but that capability is not on the wire Kandev uses, so building a
Claude "steering" path now would deliver nothing mid-turn until Anthropic
exposes it on the SDK.

Satisfies §req:success-criteria #1 (as the gated end-state), #6, #7;
§req:constraints (Claude-capable-first with graceful degradation —
sharpened by discovery to "capable-agent-first," which is Codex);
§req:quality-attributes (capability-gated, not assumed; degrade, never
lose); §req:priorities must-have #1, #5; §req:user-stories (the operator
steering a turn mid-flight).
