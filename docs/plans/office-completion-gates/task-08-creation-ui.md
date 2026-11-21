---
id: "08-creation-ui"
title: "Criteria in task creation and settings UI"
status: done
wave: 6
depends_on: ["06-creation-surfaces"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-COMPLETION-GATES-001
  - REQ-OFFICE-COMPLETION-GATES-008
acceptance_criteria:
  - AC-OFFICE-COMPLETION-GATES-001.1
  - AC-OFFICE-COMPLETION-GATES-001.2
  - AC-OFFICE-COMPLETION-GATES-008.7
  - AC-OFFICE-COMPLETION-GATES-008.8
system_design:
  - ../../specs/office/system-design/completion-gates.md
---

# Task 08: Criteria in task creation and settings UI

## Summary

The Office new-task dialog offers criteria prefilled from workspace defaults,
and Office workspace settings edit the defaults, caps and ceilings.

## ASCII UI preview

From the [plan](plan.md#ascii-ui-preview): UI-05 and UI-06.

```text
| Completion criteria  (CI at head, Review at head)        [v]  |
|   [x] CI passes at PR head                                    |
|       Protected paths  [ tests/** ] [ .github/** ] [+ Add]    |
|   [x] Review at PR head                                       |
|   (!) Authentication is off. Any process on this machine can  |
|       approve this task as you, so this review can pass       |
|       without a real review. Turn on Authentication in        |
|       Settings > System > Feature Toggles to prevent this.    |
|   Max rework rounds [ 3 ]   Max verification attempts [ 3 ]   |
```

Phone: the group stacks vertically in the dialog; the settings card fields
stack one per row with 44-pixel controls.

## In scope

- Criteria group in `new-task-dialog.tsx` (collapsed summary, prefill from
  settings, payload `completion_criteria`); draft persistence in
  `new-task-draft.ts`.
- **Completion gates** card in workspace settings with validation (positive
  integers; default not above ceiling; protected-path pattern errors from the
  API shown inline).
- Server error codes from Task 06 mapped to localized messages.
- Shared `ReviewAuthWarning` alert shown in both places when **Review at PR
  head** is selected and `state.auth.mode !== "enabled"`, linking to Feature
  Toggles; non-blocking.
- Seven locale catalogs; unit tests for payload building and validation; E2E
  desktop and mobile-chrome.

## Out of scope

- Task page section (Task 07).

## Acceptance

- Creating a task with CI (protected `tests/**`) and review criteria stores
  them; reopening the dialog in a workspace with defaults prefills them.
- Saving the settings card persists defaults, caps and ceilings and the next
  dialog uses them.
- The same flows work on a phone viewport without horizontal overflow.
- With authentication off, checking **Review at PR head** shows the warning in
  both places and creation and saving still succeed; with authentication
  enabled no warning renders (unit test on the auth-mode branch).

## Verification

```bash
(cd apps/web && pnpm exec vitest run app/office/components app/office/workspace/settings)
(cd apps/web && pnpm run typecheck && pnpm run lint && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/office/completion-gates-create.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/office/mobile-completion-gates-create.spec.ts)
```

## Files likely touched

- `apps/web/app/office/components/new-task-dialog.tsx`, `new-task-draft.ts`, new `new-task-completion-criteria.tsx`
- `apps/web/app/office/workspace/settings/components/settings-content.tsx`, new `completion-gates-card.tsx`
- `apps/web/lib/api/domains/office-extended-api.ts`, `kanban-api.ts`
- `apps/web/src/locales/*/` catalogs
- `apps/web/e2e/tests/office/*completion-gates-create.spec.ts`

## Dependencies

Task 06.

## Risks

- The new-task dialog is already dense; keep the group collapsed by default.

## Results

Done. The new-task dialog has a collapsed **Completion criteria** group
prefilled from workspace defaults; only an explicit change is sent (otherwise
the server applies defaults). The Office Preferences page has a **Completion
gates** section for default criteria, default limits and ceilings. A shared
`CriteriaEditor` shows the non-blocking authentication warning while review is
selected and auth is not enabled. Creation refusal codes map to localized
messages.

Verification: vitest, typecheck, lint, i18n checks pass; chromium and
mobile-chrome E2E pass (`completion-gates-create` and mobile twin).
