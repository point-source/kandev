---
id: "06-settings-office-section"
title: "Office workflows section in settings"
status: done
wave: 4
depends_on:
  - "03-selection-api"
  - "04-definition-save"
  - "05-export-import"
plan: "plan.md"
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-001
  - REQ-OFFICE-WORKFLOW-SELECT-002
  - REQ-OFFICE-WORKFLOW-SELECT-004
  - REQ-OFFICE-WORKFLOW-SELECT-005
  - REQ-OFFICE-WORKFLOW-SELECT-006
acceptance_criteria:
  - AC-OFFICE-WORKFLOW-SELECT-001.2
  - AC-OFFICE-WORKFLOW-SELECT-002.1
  - AC-OFFICE-WORKFLOW-SELECT-002.2
  - AC-OFFICE-WORKFLOW-SELECT-002.3
  - AC-OFFICE-WORKFLOW-SELECT-002.4
  - AC-OFFICE-WORKFLOW-SELECT-002.5
  - AC-OFFICE-WORKFLOW-SELECT-002.6
  - AC-OFFICE-WORKFLOW-SELECT-004.1
  - AC-OFFICE-WORKFLOW-SELECT-004.4
  - AC-OFFICE-WORKFLOW-SELECT-004.5
  - AC-OFFICE-WORKFLOW-SELECT-004.6
  - AC-OFFICE-WORKFLOW-SELECT-005.6
  - AC-OFFICE-WORKFLOW-SELECT-006.1
system_design:
  - ../../specs/office/system-design/office-workflow-selection.md
---

# Task 06: Office workflows section in settings

## Summary

Render an Office workflows section in Settings > Workflows for Office
workspaces: the Office workflow picker, the built-in and copy cards, duplicate
with the "no built-in updates" notice, and saving Office drafts through the
definition endpoints. Problems from the backend render as localized sentences.

## In scope

- `loadWorkspaceWorkflowsRoute`: `includeHidden: true`, `excludeOffice: false`;
  `listWorkflows` gains `excludeOffice`.
- `useWorkspaceWorkflowsPage`: Kanban list `!is_office && !hidden`; Office list
  `is_office` (replaces the `style !== "office"` filter).
- `office-workflows-section.tsx`, `use-office-workflow-selection.ts`,
  `lib/api/domains/office-workflow-api.ts`.
- Picker disabled without `SCOPE.workspaceManage`; refusal `Alert` (UI-02).
- Duplicate keeps `is_office`; notice when source `is_system` (UI-03).
- `persistWorkflowDraft` branch to POST/PUT definition for `is_office`.
- `workflows:` locale keys for section copy, badges, notice, picker, every
  problem code, in all seven catalogs.
- Phone composition (UI-05).

## Out of scope

- Stage-type selector and Office events editor (task 07).

## Acceptance

- An Office workspace shows the section with built-in and copies; a
  non-Office workspace shows none; Office workflows never render in the Kanban
  list.
- Choosing an invalid candidate shows every problem sentence and keeps the
  previous selection; a valid choice updates the selected badge.
- Saving a duplicate creates an Office copy through one definition call.

## ASCII UI preview

UI-01, UI-02, UI-03 and UI-05 from [the plan](plan.md#ascii-ui-preview).
Excerpt (UI-01, AC-002.1, AC-004.1):

```text
| Office workflows                                                          |
| Office workflow  [ Office Default (copy)                          v ]     |
| | Office Default   [Built-in] [Read-only]            [Duplicate] [...]  | |
| | Office Default (copy)   [Selected]        [Duplicate] [Export] [...]  | |
| Kanban workflows                                                          |
```

## Verification

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run --maxWorkers=1 app/settings/workspace components/settings src/settings-routes.workspace-data.test.tsx)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
```

## Files likely touched

- `apps/web/src/settings-routes.workspace-data.tsx`
- `apps/web/lib/api/domains/kanban-api.ts`, `apps/web/lib/api/domains/office-workflow-api.ts` (new)
- `apps/web/app/settings/workspace/workspace-workflows-client.tsx`
- `apps/web/app/settings/workspace/office-workflows-section.tsx` (new) and test
- `apps/web/app/settings/workspace/use-office-workflow-selection.ts` (new) and test
- `apps/web/app/settings/workspace/workflow-duplication.ts`
- `apps/web/components/settings/workflow-card-actions.ts`
- `apps/web/lib/types/http.ts`
- `apps/web/src/locales/*/workflows.json`

## Dependencies

Tasks 03, 04, 05.

## Risks

- `workspace-workflows-client.tsx` is near the file-size limit; put new logic
  in the new section component and hook.
- i18n generators touch unrelated catalogs; revert unrelated locale changes.

## Parallelism

`sequential`

## Inputs

- Design section "Settings Workflows tab"; plan UI previews.
- `/mobile-parity` skill for the phone layout.

## Results

- `pnpm exec vitest run app/settings/workspace components/settings src/settings-routes.workspace-data.test.tsx lib/ws/handlers lib/office-workflow lib/api/domains`: 500 files, 3768 tests pass.
- `pnpm run i18n:check`, `pnpm run i18n:ratchet`, `pnpm run typecheck`, `pnpm run lint`: pass.
- Deviation: the Kanban list also keeps excluding `style: office` workflows so the existing export E2E contract holds.
