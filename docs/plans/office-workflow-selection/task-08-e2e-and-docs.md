---
id: "08-e2e-and-docs"
title: "End-to-end flow and public docs"
status: done
wave: 6
depends_on:
  - "07-office-step-editor"
plan: "plan.md"
requirements:
  - REQ-OFFICE-WORKFLOW-SELECT-001
  - REQ-OFFICE-WORKFLOW-SELECT-002
  - REQ-OFFICE-WORKFLOW-SELECT-003
  - REQ-OFFICE-WORKFLOW-SELECT-004
  - REQ-OFFICE-WORKFLOW-SELECT-005
  - REQ-OFFICE-WORKFLOW-SELECT-006
  - REQ-OFFICE-WORKFLOW-SELECT-007
acceptance_criteria:
  - AC-OFFICE-WORKFLOW-SELECT-001.1
  - AC-OFFICE-WORKFLOW-SELECT-001.2
  - AC-OFFICE-WORKFLOW-SELECT-002.2
  - AC-OFFICE-WORKFLOW-SELECT-002.3
  - AC-OFFICE-WORKFLOW-SELECT-002.6
  - AC-OFFICE-WORKFLOW-SELECT-003.1
  - AC-OFFICE-WORKFLOW-SELECT-003.2
  - AC-OFFICE-WORKFLOW-SELECT-004.2
  - AC-OFFICE-WORKFLOW-SELECT-004.3
  - AC-OFFICE-WORKFLOW-SELECT-004.4
  - AC-OFFICE-WORKFLOW-SELECT-004.6
  - AC-OFFICE-WORKFLOW-SELECT-005.6
  - AC-OFFICE-WORKFLOW-SELECT-006.1
  - AC-OFFICE-WORKFLOW-SELECT-007.1
  - AC-OFFICE-WORKFLOW-SELECT-007.2
system_design:
  - ../../specs/office/system-design/office-workflow-selection.md
---

# Task 08: End-to-end flow and public docs

## Summary

Prove the issue's target workflow in Playwright on desktop and phone, and
update public documentation for Office workflow selection and the portable
`office` marker.

## In scope

- `e2e/tests/office/office-workflow-selection.spec.ts` (`chromium`): the three
  flows in the plan's E2E table.
- `e2e/tests/office/mobile-office-workflow-selection.spec.ts`
  (`mobile-chrome`): section, picker, refusal and duplicate notice at 390px
  with no document-level horizontal overflow.
- `docs/public/office.md` and `docs/public/workflow-import-export.md` via
  `/docs-maintainer`; `docs/public/feature-status.md` if it lists Office
  workflow limits.
- Promote the requirement to `active`, the design to `current`, the ADR to
  `accepted`, and the plan to `implemented` once every work order passes.

## Out of scope

- New product behavior.

## Acceptance

- Both specs pass on host runs.
- Public docs describe copying, editing, selecting, validation reasons, and
  export/import of Office workflows.

## Verification

```bash
(cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/office/office-workflow-selection.spec.ts)
(cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/office/mobile-office-workflow-selection.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check -- docs
```

## Files likely touched

- `apps/web/e2e/tests/office/office-workflow-selection.spec.ts` (new)
- `apps/web/e2e/tests/office/mobile-office-workflow-selection.spec.ts` (new)
- `apps/web/e2e/fixtures/office-fixture.ts` (second Office workspace helper)
- `docs/public/office.md`, `docs/public/workflow-import-export.md`
- Spec, design, ADR and plan status fields

## Dependencies

Task 07.

## Risks

- The first E2E flow creates a real Office task; use the mock agent and the
  existing office fixture seeding to keep it deterministic.

## Parallelism

`sequential`

## Inputs

- Plan "E2E tests" table; `/e2e` and `/mobile-parity` skills.

## Results

- `pnpm e2e:run --host --project chromium tests/office/office-workflow-selection.spec.ts`: 3 passed.
- `pnpm e2e:run --host --project mobile-chrome tests/office/mobile-office-workflow-selection.spec.ts`: 1 passed.
- Existing specs that reconfigured the built-in Office workflow with single-step writes now use an Office copy (`e2e/helpers/office-workflow-copy.ts`) or the Office > Routing endpoint: `board-done-final-step`, `mobile-board-done-final-step`, `workflow-quorum-transitions`, `office-review-author-exclusion`: all pass. `workflow-import-export`, `workflow-duplication`, `workflow-settings`: pass.
- Public docs: `docs/public/office.md`, `docs/public/workflow-import-export.md`, `docs/public/feature-status.md`; `node scripts/validate-public-docs.mjs`: pass.
