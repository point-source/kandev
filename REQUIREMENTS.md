# Fine-grained background-running status indicator

## Problem statement §req:problem-statement

Kandev already knows, per session, whether an in-flight turn is the foreground
agent actively generating output or is idle and only held open by spawned
background work — a subagent, a backgrounded shell, or an active monitor
(the fine-grained busy signal, ADR-0043). That signal is emitted and, inside an
open task's session-switcher dropdown, it is rendered distinctly.

But the status indicators an operator actually relies on to judge "is this task
still working or is it done?" do **not** reflect it uniformly. On the
at-a-glance surfaces — the board/kanban card, the task list rows, the
graph/swimlane nodes — and on the sidebar / session-tab / global "something is
running" indicator, a session whose foreground turn has finished while its
background work is still running shows the **same "done" affordance** (a check)
as a task that is genuinely finished. The session-reopen menu drops the signal
too.

So the operator cannot trust the indicator: a task that is still doing work
looks identical to a finished one everywhere except one buried dropdown. The
indicator is coarser than the signal beneath it, and the surfaces disagree with
each other about what a given state means.

## Success criteria §req:success-criteria

Each criterion is observable from the product's status surfaces.

- On **every** surface that displays a task or session status (board/kanban
  card, task list row, graph/swimlane node, open-task header, sidebar / session
  tab / global running indicator, session switcher, session-reopen menu), a
  task/session whose foreground turn is idle while spawned background work is
  still running shows an indicator that is visually distinct from **both** the
  "done" indicator **and** the "actively generating" (foreground) indicator —
  three states an operator can tell apart at a glance.
- No status surface shows a "done" affordance for a task/session while any
  background work it spawned is still running. The not-done indicator persists
  until **all** such background work completes — an agent that has already
  replied but left a shell or monitor running does not read as "done".
- For a task running more than one session at once, its single task-level
  indicator follows most-active-wins: it shows the foreground "generating"
  affordance if any session is generating; it shows the background affordance
  only when no session is generating but at least one has background work
  running; it shows "done" only when neither is true.
- The same underlying state produces the same visual meaning on every surface —
  an operator does not have to remember which surface is trustworthy.
- The indicator updates promptly as a session moves between generating,
  working-in-background, and done, without a manual refresh; a freshly loaded
  page or a second tab shows the correct state immediately.
- The established foreground "actively generating" affordance is unchanged;
  operators who already recognize it keep recognizing it.

## User stories §req:user-stories

- As an operator scanning the board or task list, I can tell a task that is
  still doing background work apart from a finished one **without opening it**.
- As an operator, when an agent has posted its answer but a shell or monitor it
  launched is still running, the task does **not** look "done" until that
  background work finishes.
- As an operator whose task is running several sessions at once, the single
  task indicator reflects the most-active state (generating over background
  over done), so I am never told "done" while work continues.
- As an operator glancing at the sidebar / session tabs, the
  background-running state is distinct from both the active-generating
  indicator and the done state.
- As an operator, wherever a status icon appears it means the same thing, so I
  do not have to learn which surface to trust.

## Quality attributes §req:quality-attributes

- **Consistency / truthfulness**: the primary attribute — every status surface
  reports the same three-state distinction for the same underlying state, and
  none ever falsely reads "done" while work is happening.
- **Live updates**: transitions between generating, background, and done are
  reflected promptly; a stale "done" that only clears on refresh is a defect.
- **At-a-glance legibility**: the three states are distinguishable quickly in a
  dense scan of many tasks, and on mobile as well as desktop.
- **Not color-alone**: the background state is separable from done and from
  generating by shape or motion, not hue only, so it remains distinguishable
  for color-vision-deficient operators.
- **Safe defaults**: when the fine-grained state is unknown or unavailable
  (e.g. after a backend restart resets the in-memory signal), a surface must
  fall back to a state that does not falsely claim "done" while a turn is open.

## Constraints §req:constraints

- The distinction is only as fine-grained as the underlying signal. Recognition
  of background work is best-effort and currently Claude-shaped (subagent /
  backgrounded shell / active monitor); agents whose background activity is not
  recognized keep today's behavior. Broadening what counts as "background work"
  is out of scope.
- This work concerns the **status indicator** only. The composer input-gating
  behavior ("you may type while background runs") is handled separately and is
  not part of these requirements; the background indicator here only needs to
  communicate "still running", not "you can type".
- The fine-grained signal is in-memory and not persisted (by design); a fresh
  page load / second tab must remain correct via the existing boot payload and
  session DTOs, and this work must not regress that.
- The existing foreground "generating" affordance must not be restyled or
  repurposed; this is an additive, consistency change across surfaces.

## Priorities §req:priorities

The driving need the operator named is truthful, consistent status across every
surface — not preventing a specific catastrophic mistake, but ending the
situation where the UI contradicts a signal it already computes and disagrees
with itself surface to surface.

The highest-impact, highest-confidence, lowest-risk work is therefore
propagating the already-emitted substate to the surfaces that currently ignore
it: the task-level at-a-glance indicators (board card, task list, graph/swimlane
nodes), the sidebar / session-tab / global running indicator, and the
session-reopen menu that drops the parameter. Confidence is high and risk is low
because the signal is already produced end-to-end and one surface already
renders all three states correctly — this is consistency work, not new
mechanism.

The multi-session most-active-wins aggregation at the task level is a smaller,
well-scoped addition layered on top. Broadening background-work recognition to
non-Claude agents is explicitly deferred and not part of this effort.
