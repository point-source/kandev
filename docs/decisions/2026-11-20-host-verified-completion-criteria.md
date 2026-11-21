# ADR-2026-11-20-host-verified-completion-criteria: Kandev verifies CI and review criteria in process

**Status:** superseded by ADR-2026-11-21-completion-gates-people-act-on-exceptions
**Date:** 2026-11-20
**Area:** backend

Superseded by
[ADR-2026-11-21-completion-gates-people-act-on-exceptions](2026-11-21-completion-gates-people-act-on-exceptions.md),
which keeps the in-process verifier design and replaces the person review
shortcut, the confidence levels and the criterion kind names.

## Context

Task completion gates store criteria and evidence, but any authorized caller
can submit evidence. With authentication off, every HTTP caller is the
synthetic admin, so an agent run on a local executor can verify its own
criterion. Office needs CI and review criteria that the worker cannot vouch
for. [Issue #5351](https://github.com/kdlbs/kandev/issues/5351)

## Decision

Add host-verified criterion kinds to the task-owned gate. Their evidence and
results come only from task-service commands that in-process Office verifiers
call: a CI verifier that reads PR checks at the head through the GitHub
service, and a review verifier that reads reviewer decisions pinned to the
heads. The HTTP, plugin Host, and MCP evidence paths refuse these kinds by
kind, not by identity. Proof uses one subject, the set of linked PR heads, so
any push, link, or detach makes it stale.

The task system keeps the primitive, the guard, and the audit history. Office
keeps verifier policy, routing, limits, and UI in
`internal/office/completiongates`.

## Alternatives considered

- **A plugin verifier.** It would have to mirror Office decisions and PR checks
  through events, and it cannot tell a task's own run from a human when
  authentication is off.
- **Identity-based fencing on the existing evidence route.** It fails with
  authentication off, because every caller has the same identity.
- **One evidence subject per PR.** Multi-repository tasks would need one
  criterion per PR, and PRs linked after creation would have no criterion.

## Consequences

- The fence holds with authentication on or off for CI criteria.
- Human approvals still count for review criteria, so with authentication off
  a local process can approve as a person. The UI warns when a review
  criterion is configured with authentication off; enabling authentication
  remains the user's choice.
- New criterion kinds that need independent verification follow the same
  pattern: a reserved kind and an in-process verifier.

See the [Office completion gates design](../specs/office/system-design/completion-gates.md).
