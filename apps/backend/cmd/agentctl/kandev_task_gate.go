package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
)

// taskGate dispatches completion-gate subcommands.
func taskGate(args []string) int {
	if len(args) == 0 || args[0] != "extend" {
		cliError("usage: agentctl kandev task gate extend --criterion <id> [--task <id>] [--attempts <n>] [--rework <n>] [--reason <text>]")
		return 1
	}
	return taskGateExtend(args[1:])
}

// taskGateExtend raises a held criterion's caps within the workspace ceiling.
// Only the criterion's owner agent or a CEO agent may do this.
func taskGateExtend(args []string) int {
	fs := flag.NewFlagSet("task gate extend", flag.ContinueOnError)
	taskID := fs.String("task", os.Getenv("KANDEV_TASK_ID"), "Task ID (defaults to $KANDEV_TASK_ID)")
	criterion := fs.String("criterion", "", "Criterion ID, for example ci_at_head (required)")
	attempts := fs.Int("attempts", 0, "New verification-attempt cap")
	rework := fs.Int("rework", 0, "New rework-round cap")
	reason := fs.String("reason", "", "Why the cap is raised")
	if err := fs.Parse(args); err != nil {
		cliError("parse flags: %v", err)
		return 1
	}
	if fs.NArg() > 0 {
		cliError("unexpected arguments: %s", strings.Join(fs.Args(), " "))
		return 1
	}
	if strings.TrimSpace(*taskID) == "" || strings.TrimSpace(*criterion) == "" {
		cliError("--task and --criterion are required")
		return 1
	}
	if *attempts <= 0 && *rework <= 0 {
		cliError("--attempts or --rework must be a positive number")
		return 1
	}
	client, err := newKandevClient()
	if err != nil {
		cliError("%v", err)
		return 1
	}
	path := fmt.Sprintf("/api/v1/office/tasks/%s/completion-gate/criteria/%s/extend",
		url.PathEscape(*taskID), url.PathEscape(*criterion))
	body, status, err := client.do(http.MethodPost, path, map[string]any{
		"max_verification_attempts": *attempts, "max_rework_rounds": *rework, "reason": *reason,
	})
	return handleResponse(body, status, err)
}

// criteriaFlags collects completion criteria for task create.
type criteriaFlags struct {
	require     stringList
	protect     stringList
	maxRework   *int
	maxAttempts *int
}

type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func registerCriteriaFlags(fs *flag.FlagSet) *criteriaFlags {
	flags := &criteriaFlags{}
	fs.Var(&flags.require, "require", "Completion criterion to require: ci-at-head or review-at-head (repeatable); omit to apply the workspace default")
	fs.Var(&flags.protect, "protect", "Protected path glob for ci-at-head, for example tests/** (repeatable)")
	flags.maxRework = fs.Int("max-rework", 0, "Rework-round cap for the required criteria")
	flags.maxAttempts = fs.Int("max-attempts", 0, "Verification-attempt cap for the required criteria")
	return flags
}

var criterionFlagKinds = map[string]string{"ci-at-head": "ci_at_head", "review-at-head": "review_at_head"}

// build returns nil when no criterion was required, so the workspace default
// applies.
func (f *criteriaFlags) build() ([]map[string]any, error) {
	if len(f.require) == 0 {
		if len(f.protect) > 0 || *f.maxRework > 0 || *f.maxAttempts > 0 {
			return nil, fmt.Errorf("--protect, --max-rework and --max-attempts need --require")
		}
		return nil, nil
	}
	criteria := make([]map[string]any, 0, len(f.require))
	for _, name := range f.require {
		kind, ok := criterionFlagKinds[strings.TrimSpace(name)]
		if !ok {
			return nil, fmt.Errorf("--require must be ci-at-head or review-at-head, got %q", name)
		}
		criterion := map[string]any{"kind": kind}
		if kind == "ci_at_head" && len(f.protect) > 0 {
			criterion["protected_paths"] = []string(f.protect)
		}
		if *f.maxRework > 0 {
			criterion["max_rework_rounds"] = *f.maxRework
		}
		if *f.maxAttempts > 0 {
			criterion["max_verification_attempts"] = *f.maxAttempts
		}
		criteria = append(criteria, criterion)
	}
	return criteria, nil
}
