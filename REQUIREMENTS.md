# Requirements — Mid-turn steering: talk to an agent while it is still generating

> Source: GitHub issue [#1607](https://github.com/kdlbs/kandev/issues/1607)
> — "Support mid-turn input for agents that allow it (Claude-style steering)".
>
> Provenance / scope pivot (2026-07-10): this file previously held the
> requirements for the #1597 executor-row-desync work and its adjacent
> "accept input while the foreground turn is idle on background work" behavior
> (call it **scope A**). That scope is considered functioning on current `main`
> via upstream fixes #1600 / #1602 and is documented in the #1597 branches and
> git history. This task deliberately targets the *other* half — **scope B**:
> steering a turn that is genuinely, actively generating. Scope A is out of
> scope here (see §req:constraints).

## Problem statement §req:problem-statement

An operator drives Claude agent sessions in Kandev, watching a turn unfold —
text, tool calls, sometimes long-running work. Partway through, the operator
often realizes they want to *add* something to the turn in flight: a
correction ("use the config loader, not the raw file"), a clarification
("only the backend package"), or a redirection ("stop — wrong file"). They do
not want to end the turn; they want to nudge it while it runs.

Claude Code natively supports this. You type while the model is generating; the
message is accepted **inline**, the model addresses it at its next opportunity,
and — crucially — you can **edit that message up until the moment the model
takes it**. This is *steering*: the operator's words influence the turn that is
already happening.

Kandev today does not steer. Input typed during an active turn goes into a local
queue that only sends once the agent has **fully halted**. In the operator's
words: *"current behavior is it goes into a queue but doesn't send until the
agent halts, which is not the same as the agent permitting steering."* The
consequence is that the operator cannot influence an in-flight turn at all. If a
long turn — tool calls, subagents, background work — is heading the wrong way,
the operator's only lever is to cancel/interrupt the *entire* turn, throwing
away the in-progress work, rather than nudging it back on course. The typed
message just waits, inert, until the turn the operator wanted to change is
already over.

This capability is **not uniform across agents**. Claude accepts a message
mid-turn; other agents reachable through the same protocol (for example codex,
opencode, gemini) may not accept a second message while a turn is in flight.
Steering therefore cannot be assumed everywhere — where an agent cannot steer,
it must keep behaving *exactly* as it does today, with no new behavior and no
risk of dropped or misordered messages.

A separate, adjacent problem — a session that is merely **idle waiting on
spawned background work** (a monitor watch, a backgrounded shell) being wrongly
reported "busy" — is **not** this document's problem. That is scope A, treated
as already resolved upstream. This document is strictly about influencing a turn
that is *actively generating*.

## Success criteria §req:success-criteria

Observable from the app, using a Claude agent unless noted:

1. **A message typed while the agent is actively generating reaches the agent
   during that same turn.** It is delivered at the agent's next steering
   opportunity rather than waiting for the turn to fully end, and the operator
   can see the agent take the steer up within the ongoing turn — no cancel, no
   waiting for the turn to finish.
2. **A steered message can be edited until the agent consumes it.** From the
   moment it is sent to the moment the agent takes it, the operator can change
   its text; the version the agent receives is the latest edit.
3. **A steered message can be retracted until the agent consumes it.** Before
   the agent takes it, the operator can pull it back so that nothing is sent to
   the turn.
4. **A steered message the turn never consumes is never lost.** If the turn ends
   before the agent takes the message, it is delivered seamlessly as the
   operator's next message — the same outcome as today's queue — and is never
   silently dropped.
5. **Agents that do not support steering behave exactly as today.** For a
   non-steering agent the message queues and is delivered when the agent halts;
   there is no new behavior, no regression, and no dropped or reordered message.
6. **The operator can tell whether their message will steer or wait.** The input
   surface makes clear, for the current agent and turn, whether a typed message
   will steer the live turn or sit in the queue until the turn ends — so the
   operator is not surprised about when the agent will see it.

## User stories §req:user-stories

- **As an operator watching my Claude agent head down the wrong path mid-turn**,
  I want to type a course-correction and have it reach the agent during the
  turn, so I can nudge it without cancelling the turn and losing its in-progress
  work. *(→ §req:success-criteria #1)*

- **As an operator who fired off a steer too hastily**, I want to edit or retract
  it before the agent reads it, so a typo or a change of mind doesn't get baked
  into the turn. *(→ §req:success-criteria #2, #3)*

- **As an operator whose steer wasn't picked up before the turn ended**, I want
  it delivered automatically as my next message, so I never have to retype it and
  it is never lost. *(→ §req:success-criteria #4)*

- **As an operator using a non-Claude agent**, I want input to behave exactly as
  it does today, so nothing I rely on regresses. *(→ §req:success-criteria #5)*

- **As an operator**, I want to know at a glance whether my message will steer
  the live turn or wait for it to finish, so the timing of when the agent sees my
  words is never a surprise. *(→ §req:success-criteria #6)*

## Quality attributes §req:quality-attributes

- **Reliability and ordering.** Steered messages must reach the agent in the
  order the operator sent them and must never corrupt, duplicate, or reorder the
  in-flight turn. If a steer cannot be delivered live, it must fall back to the
  queue — degrade, never lose.
- **No regression.** Existing between-turns messaging, the current queue
  behavior, and the scope-A "accept input while idle on background work"
  behavior all remain unchanged. Non-steering agents are byte-for-byte
  unaffected.
- **Capability-gated, not assumed.** Steering is enabled only where the agent
  genuinely supports mid-turn input. That support is detected in code and is
  verifiable — never hardcoded optimism that could misfire on an agent that
  cannot steer.
- **Responsiveness.** A steer should reach the agent promptly at its next
  opportunity; the operator should not perceive a long lag between sending the
  steer and the agent acknowledging it, subject to the agent's own turn cadence.
- **A clear consumption cutoff.** The edit/retract window must have an
  unambiguous end: once the agent has taken the message it is locked. The
  operator can never edit or retract a message the agent has already acted on,
  and can always tell which side of that line a message is on.

## Constraints §req:constraints

- **Claude-capable agents first; graceful degradation for the rest.** Only
  agents that truly support mid-turn input get the live-steer path; every other
  agent keeps today's queue-until-halt behavior. *(Operator decision.)*
- **Unconsumed steer auto-delivers as the next message.** A steer the turn never
  takes becomes the next prompt automatically, matching today's queue outcome —
  it is not held back for a separate confirmation. *(Operator decision.)*
- **Reuse the existing queue/composer surface where possible.** The steer path
  should feel like the queue the operator already uses, upgraded to send into the
  live turn — not a parallel, unfamiliar mechanism. *(Operator suggestion; the
  mechanism itself is left to the Spec step.)*
- **Scope A is out of scope.** Accepting input while the foreground turn is idle
  on background work is treated as functioning via upstream #1600 / #1602 and is
  not re-addressed here. This task proceeds straight to steering; if scope A is
  later found to have regressed, that is handled separately. *(Operator
  decision: proceed straight to B without re-verifying A.)*
- **Backend conventions apply.** Per `apps/backend/CLAUDE.md`: agent-capability
  detection is gated in code; every changed behavior carries regression tests;
  commits follow Conventional Commits.

## Priorities §req:priorities

Ordered by operator impact.

**Must have**

1. **Live steering for Claude.** A message typed during an active turn reaches
   the agent within that turn, not after it halts. This is the core value of the
   feature. *(§req:success-criteria #1)*
2. **Editable until consumed.** The operator can revise a sent steer up until the
   agent takes it — this is central to matching Claude Code's native steering,
   not a bonus. *(§req:success-criteria #2)*
3. **Never lost.** A steer the turn doesn't consume is delivered as the next
   message rather than dropped. *(§req:success-criteria #4)*
4. **No regression for non-steering agents.** Agents that can't steer keep exactly
   today's behavior. *(§req:success-criteria #5)*
5. **Capability detection.** Steering is offered only where the agent supports it,
   proven in code. *(§req:success-criteria #5, #6)*

**Nice to have**

6. **Retract / pull-back until consumed.** The operator can cancel a steer before
   the agent takes it. Strongly desired, but the operator flagged it as
   feasibility-dependent ("optionally pull it back"), so it sits just below the
   editable-until-consumed core. *(§req:success-criteria #3)*
7. **Explicit steer-vs-queue affordance.** The composer visibly signals whether a
   message will steer the live turn or wait for it to end. *(§req:success-criteria
   #6)*
