---
id: "07-gate-status-ui"
title: "Gate status and recovery UI"
status: done
wave: 5
depends_on: ["05-limits-holds-escalation"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-COMPLETION-GATES-003
  - REQ-OFFICE-COMPLETION-GATES-008
acceptance_criteria:
  - AC-OFFICE-COMPLETION-GATES-003.2
  - AC-OFFICE-COMPLETION-GATES-008.1
  - AC-OFFICE-COMPLETION-GATES-008.2
  - AC-OFFICE-COMPLETION-GATES-008.3
  - AC-OFFICE-COMPLETION-GATES-008.4
  - AC-OFFICE-COMPLETION-GATES-008.5
  - AC-OFFICE-COMPLETION-GATES-008.6
system_design:
  - ../../specs/office/system-design/completion-gates.md
---

# Task 07: Gate status and recovery UI

## Summary

The Office task page shows a **Completion criteria** section for gated tasks,
with override, protected-path and limit actions, and the Inbox shows the matching
items. Desktop and phone.

## ASCII UI preview

From the [plan](plan.md#ascii-ui-preview): UI-01, UI-02, UI-03, UI-04 and UI-07.

```text
| Completion criteria                 Weakest: judgement        |
| CI at PR head          [|| Held]    machine                   |
|   protected_files_changed: tests/webhook_test.go               |
|   [ Allow for this commit ]  [ Send back ]                    |
| Review at PR head      [! Stale]    judgement                 |
|   Approved at 9f8e7d6, head is now a1b2c3d                    |
|                                    [ Override completion ]    |
```

Phone: same order in the single main column; actions full width, 44-pixel
targets, dialogs as bottom sheets.

## In scope

- `CompletionCriteriaSection` in `OfficeSimplePane.tsx` main column, rendered
  only when the status summary reports criteria; refetch on `task.updated` and
  summary gate revision.
- Row rendering per UI-01/02/04; dialogs for override (existing move override
  request), Allow, Send back, Allow more attempts.
- Inbox item renderers for the two kinds with the same dialogs.
- API client functions; seven locale catalogs; no em dashes in copy.
- Unit tests for row state mapping and the API client; E2E desktop and
  mobile-chrome.

## Out of scope

- Generic task page (unchanged). Creation and settings UI (Task 08).

## Acceptance

- A seeded gated task with mock checks shows failed, held and passed states
  live without reload; Allow and Send back act and the Inbox item disappears.
- Override with a reason completes a blocked task and the section shows it was
  overridden and by whom.
- A task without criteria shows no section and makes no gate request
  (network assertion in E2E).

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/task/simple lib/api/domains/task-completion-gates-api)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/office/completion-gates-ci.spec.ts tests/office/completion-gates-override.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/office/mobile-completion-gates-ci.spec.ts tests/office/mobile-completion-gates-override.spec.ts)
```

## Files likely touched

- New `apps/web/components/task/simple/completion-criteria-section.tsx` and dialogs, tests
- `apps/web/components/task/simple/OfficeSimplePane.tsx`
- `apps/web/app/office/inbox/` item components
- `apps/web/lib/api/domains/task-completion-gates-api.ts`, `office-extended-api.ts`
- `apps/web/src/locales/*/` catalogs
- `apps/web/e2e/tests/office/*completion-gates*.spec.ts`, `apps/web/e2e/helpers/api-client.ts`

## Dependencies

Task 05 (routes and Inbox kinds).

## Risks

- The Office page has no phone-specific layout; check the section does not
  cause horizontal overflow with long file paths (wrap and truncate).

## Results

Done. `CompletionCriteriaSection` (main column of `OfficeSimplePane`) loads
the gate and history only when the live status summary reports criteria, and
refetches when the summary changes; the gate summary now carries
`failed_count` and `cannot_evaluate_count`, so every result change republishes
it. Rows show kind, status, reason and detail, short heads, verifier,
confidence and counts; Allow, Send back, Allow more attempts and Override
open one responsive confirmation (dialog on desktop, bottom sheet on phones).
Inbox rows for both new types reuse the dialogs and appear under **Mine**. Copy
is in seven locales.

E2E found a startup-order bug: the verifier was started before Office services
were constructed, so it never ran. Start/stop now sit right after
construction, guarded by `TestCompletionGatesStartAfterConstruction`.

Verification: vitest (helpers, API client, Office and task components),
typecheck, lint and i18n checks pass; chromium and mobile-chrome E2E pass
(`completion-gates-ci`, `completion-gates-override` and mobile twins).
