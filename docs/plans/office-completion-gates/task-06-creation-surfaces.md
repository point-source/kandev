---
id: "06-creation-surfaces"
title: "Creation surfaces and workspace defaults"
status: done
wave: 5
depends_on: ["01-host-verified-criteria", "05-caps-holds-escalation"]
plan: "plan.md"
requirements:
  - REQ-OFFICE-COMPLETION-GATES-001
  - REQ-OFFICE-COMPLETION-GATES-007
acceptance_criteria:
  - AC-OFFICE-COMPLETION-GATES-001.1
  - AC-OFFICE-COMPLETION-GATES-001.2
  - AC-OFFICE-COMPLETION-GATES-001.3
  - AC-OFFICE-COMPLETION-GATES-001.4
  - AC-OFFICE-COMPLETION-GATES-001.5
  - AC-OFFICE-COMPLETION-GATES-007.2
system_design:
  - ../../specs/office/system-design/completion-gates.md
---

# Task 06: Creation surfaces and workspace defaults

## Summary

Every Office creation path can attach criteria, and a workspace default
applies when a request names none. Agents and routines cannot drop a default
or exceed a ceiling.

## In scope

- `completiongates.ResolveCreateCriteria` with the rules in the design
  (defaults, union for agents and routines, repository, Office-task and
  review-stage checks, ceilings, protected-path validation, owner).
- HTTP `POST /api/v1/tasks` `completion_criteria` for Office requests.
- CLI `kandev task create --require/--protect/--max-rework/--max-attempts`
  through `office/runtime` `CreateTaskInput`.
- MCP `create_task_kandev` `completion_criteria` and tool schema.
- Routine `taskTemplate.completion_criteria` through `RoutineTaskOptions`.
- Default criteria in the Task 05 settings API.
- Skipped-default activity entry.

## Out of scope

- UI (Task 08).

## Acceptance

- Each of the four surfaces creates a task whose first committed version has
  the requested criteria; an agent request without criteria gets the workspace
  default and cannot remove it.
- A CI criterion on a task without a repository, or a review criterion
  without a review stage, is refused with its code and no task is created.
- Office tasks created without criteria and without a default behave as before
  (existing Office creation tests pass unchanged).

## Verification

```bash
(cd apps/backend && go test -trimpath ./internal/office/completiongates/... ./internal/office/runtime/... ./internal/office/routines/... ./internal/mcp/... ./internal/task/handlers/... ./internal/backendapp/... -run 'Create|CompletionCriteria|Routine')
(cd apps/backend && go test -trimpath ./cmd/agentctl/... -run 'TaskCreate')
bash scripts/lint-go-changed $(git diff --name-only origin/main -- '*.go')
```

## Files likely touched

- New `apps/backend/internal/office/completiongates/create.go`
- `apps/backend/internal/task/handlers/task_http_handlers.go`
- `apps/backend/internal/office/runtime/actions.go`, `handler.go`
- `apps/backend/cmd/agentctl/kandev_task.go`
- `apps/backend/internal/mcp/handlers/create_task_mode.go`, `handlers.go`, `internal/mcp/server/server.go`
- `apps/backend/internal/office/routines/service.go`, `task_context.go`
- `apps/backend/internal/backendapp/adapters_office.go`

## Dependencies

Tasks 01 and 05.

## Risks

- The HTTP create path serves Kanban too; criteria must be refused, not
  ignored, on non-Office requests.

## Results

Done. The task service gained a `CompletionCriteriaResolver` hook, called in
`prepareTaskForCreation` for Office requests after repositories and the Office
workflow are resolved; `completiongates.Service.ResolveCreateCriteria`
implements it (defaults, union for agents and routines, repository and
review-stage checks, ceilings, owner). All four surfaces converge on it: HTTP
`completion_criteria`, MCP `create_task_kandev.completion_criteria` (an
agent's own session is attributed to an agent even though dispatch carries the
owner's identity), CLI `kandev task create --require/--protect/--max-rework/
--max-attempts` through `AgentTaskRelations`, and routine templates through
`RoutineTaskOptions`. Criteria on a non-Office task are refused. Refusals are a
typed `CompletionCriteriaRefusal` with stable codes, mapped to 400 on the HTTP
and runtime create paths.

Deviation: a workspace default that cannot apply is skipped and logged, not
written to the task's activity, because the activity entry would need the task
ID before the task exists. Recorded for follow-up.

Verification: Office, MCP, task handlers and service, agentctl and backendapp
packages pass; changed-Go lint reports 0 issues.
