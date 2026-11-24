---
id: "07-office-step-editor"
title: "Office step editor controls"
status: done
wave: 5
depends_on:
  - "06-settings-office-section"
plan: "plan.md"
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-003
acceptance_criteria:
  - AC-OFFICE-WORKFLOW-SELECT-003.1
  - AC-OFFICE-WORKFLOW-SELECT-003.2
  - AC-OFFICE-WORKFLOW-SELECT-003.3
  - AC-OFFICE-WORKFLOW-SELECT-003.4
system_design:
  - ../../specs/office/system-design/office-workflow-selection.md
---

# Task 07: Office step editor controls

## Summary

In Office copies, replace the hold checkbox with a stage-type selector and add
an advanced Office events YAML editor per step. The built-in Office workflow
opens read-only.

## In scope

- `StepConfigPanel`: stage-type `Select` (work, review, approval, custom,
  hold) when `workflow.is_office`; hold checkbox unchanged for Kanban.
- `workflow-step-office-events.tsx`: monospace `Textarea`, `yaml` serialize
  and parse, step references shown as step names and mapped back to IDs or
  draft keys; inline parse errors block Save; server `invalid_events` problems
  attach to the step.
- Refuse duplicate step names in Office workflows while the events editor is
  in use (names must map back uniquely).
- Built-in renders with `readOnly`.
- Locale keys in all seven catalogs.

## Out of scope

- Structured per-trigger forms; moving **Review on a different provider**.

## Acceptance

- Editing YAML to add `run_code_review` with `agent_profile_id` and saving
  persists it, visible after reload.
- Invalid YAML or an unknown action shows a located error and Save writes
  nothing.

## ASCII UI preview

UI-04 and UI-05 from [the plan](plan.md#ascii-ui-preview). Excerpt (UI-04,
AC-003.1, AC-003.2):

```text
| Stage type [ Review                  v ]                                |
| v Office events (advanced)                                              |
| | on_enter:                                                           | |
| |   - type: run_code_review                                           | |
| x Line 7: unknown action "ensure_seat" in on_enter.                     |
```

## Verification

```bash
(cd apps/web && pnpm exec vitest run --maxWorkers=1 components/settings app/settings/workspace)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run lint)
```

## Files likely touched

- `apps/web/components/settings/workflow-pipeline-editor-panels.tsx`
- `apps/web/components/settings/workflow-step-office-events.tsx` (new) and test
- `apps/web/components/settings/workflow-pipeline-editor-office.test.tsx` (new)
- `apps/web/components/settings/workflow-step-draft-equality.ts`
- `apps/web/src/locales/*/workflows.json`

## Dependencies

Task 06.

## Risks

- `workflow-pipeline-editor-panels.tsx` is 637 lines; put new UI in new files.
- Dirty-state equality must treat semantically equal YAML as unchanged.

## Parallelism

`sequential`

## Inputs

- Design section "Office workflow editor"; plan UI-04 and UI-05.

## Results

- `components/settings/workflow-step-office-controls.test.tsx`, `lib/office-workflow/office-events-yaml.test.ts`: pass.
- `pnpm run i18n:check`, `typecheck`, `lint`: pass.
- Deviation: action config keys are not validated (the client checks syntax, triggers, action types and step references; the backend re-checks action types and references), matching the rest of the workflow editor.
