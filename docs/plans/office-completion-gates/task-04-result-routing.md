---
id: "04-result-routing"
title: "Result routing"
status: done
wave: 3
depends_on: ["02-ci-verifier", "03-review-at-head"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-COMPLETION-GATES-003
  - REQ-OFFICE-COMPLETION-GATES-004
  - REQ-OFFICE-COMPLETION-GATES-006
acceptance_criteria:
  - AC-OFFICE-COMPLETION-GATES-003.4
  - AC-OFFICE-COMPLETION-GATES-004.3
  - AC-OFFICE-COMPLETION-GATES-004.5
  - AC-OFFICE-COMPLETION-GATES-006.1
  - AC-OFFICE-COMPLETION-GATES-006.2
  - AC-OFFICE-COMPLETION-GATES-006.3
  - AC-OFFICE-COMPLETION-GATES-006.4
system_design:
  - ../../specs/office/system-design/completion-gates.md
---

# Task 04: Result routing

## Summary

Turn results into workflow movement: a CI failure returns the task to the work
stage, a stale review returns it to the review stage, a rejection counts as a
rework round, and passing criteria retry the completing transition.

## In scope

- `completiongates.Router`: reacts to changed evaluation results and to PR
  head changes; moves through the task service move API with a system actor
  and reason `completion_gate_*`.
- CI `fail` past the work stage: rework increment in the evaluation command,
  move to the work stage. In the work stage: no move.
- `cannot_evaluate`: attempt increment only.
- Stale review: return from approval stage or later to the review stage;
  re-entry when already in the review stage via step-entry dispatch with a
  fresh entry identity (confirm the mechanism first; record it in Results).
- Rejection counting from the task step-transition ledger, keyed by entry ID.
- Protected-path **Send back** (`action: send_back` on the Task 02 route).
- All-pass retry: engine guarded-transition re-evaluation for the current step.
- Prompt section **Completion criteria** in `office/service/prompt_builder.go`.
- Metric `office_completion_gate_rework_total{kind}`.

## Out of scope

- Limits enforcement beyond counting, holds, escalation (Task 05).

## Acceptance

- A gated task in the approval stage whose CI fails moves to the work stage,
  its assignee is woken with the failing checks in the prompt, and
  `rework_count` is 1; redelivering the same PR update changes nothing.
- A push after review approval returns the task from approval to review, with
  decisions cleared and reviewers woken.
- When the last criterion passes on a step whose quorum is already met, the
  task enters the completing step without another decision.

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/office/completiongates/... ./internal/office/service/... ./internal/orchestrator/... -run 'CompletionGate|Router|PromptCompletion')
(cd apps/backend && go test -trimpath ./internal/backendapp/... -run 'CompletionGate')
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Files likely touched

- New `apps/backend/internal/office/completiongates/router.go`, `rejections.go`
- `apps/backend/internal/office/service/prompt_builder.go`
- `apps/backend/internal/office/dashboard/` (send-back action)
- `apps/backend/internal/backendapp/` wiring and an end-to-end Go test

## Dependencies

Tasks 02 and 03.

## Risks

- Moves trigger step-entry actions and run queuing; use the existing move API
  so WIP admission, ledgers and events stay consistent.
- Review-stage re-entry may need a small engine entry point; keep it generic.

## Results

Done, with one design change recorded in the system design. Re-entering the
review stage while the task is already there needed a new engine entry point;
instead, approvals pinned to old heads simply stop counting, and a task that
leaves the review stage forward with the review criterion unmet is returned
there (counted). The engine gained `ReevaluateGuardedTransitions` and the
dispatcher `RetryGuardedTransitions` for the all-pass retry. Rework rounds are
counted by a keyed task command (`count_rework`, new `rework_keys_json` column
holding the last 32 counted keys, with a claimed flag in the result), so
redelivery never double counts. A task leaving the work stage while CI fails at
the live heads is returned too, so reviewers never review failing CI. Send back
releases the protected hold and returns the task to the work stage. The prompt
builder renders unmet criteria through `SetCompletionGateReader`.

Verification: completiongates router tests, Office service prompt test,
workflow engine, Office, workflow and task repository packages pass; changed-Go
lint is clean after splitting two functions. A backendapp end-to-end Go test
was not added; the browser E2E in Task 07 exercises the CI-fail return path
against the mock GitHub client.
