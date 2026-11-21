package main

import (
	"flag"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// taskGate dispatches completion-gate subcommands.
func taskGate(args []string) int {
	if len(args) == 0 || args[0] != "extend" {
		cliError("usage: agentctl kandev task gate extend --criterion <id> [--task <id>] [--verification-limit <n>] [--rework-limit <n>] [--reason <text>]")
		return 1
	}
	return taskGateExtend(args[1:])
}

// taskGateExtend raises a held criterion's limits within the workspace ceiling.
// Only the criterion's owner agent or a CEO agent may do this.
func taskGateExtend(args []string) int {
	fs := flag.NewFlagSet("task gate extend", flag.ContinueOnError)
	taskID := fs.String("task", os.Getenv("KANDEV_TASK_ID"), "Task ID (defaults to $KANDEV_TASK_ID)")
	criterion := fs.String("criterion", "", "Criterion ID, for example github_check (required)")
	attempts := fs.Int("verification-limit", 0, "New verification limit (attempts)")
	rework := fs.Int("rework-limit", 0, "New rework limit (rounds)")
	reason := fs.String("reason", "", "Why the limit is raised")
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
		cliError("--verification-limit or --rework-limit must be a positive number")
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
		"verification_limit": *attempts, "rework_limit": *rework, "reason": *reason,
	})
	return handleResponse(body, status, err)
}

// criteriaFlags collects completion criteria for task create.
type criteriaFlags struct {
	require           stringList
	protect           stringList
	reworkLimit       *int
	verificationLimit *int
	reviewWaitLimit   *time.Duration
}

type stringList []string

func (l *stringList) String() string { return strings.Join(*l, ",") }

func (l *stringList) Set(value string) error {
	*l = append(*l, value)
	return nil
}

func registerCriteriaFlags(fs *flag.FlagSet) *criteriaFlags {
	flags := &criteriaFlags{}
	fs.Var(&flags.require, "require", "Completion criterion to require: github-check or review (repeatable); omit to apply the workspace default")
	fs.Var(&flags.protect, "protect", "Protected path glob for github-check, for example tests/** (repeatable)")
	flags.reworkLimit = fs.Int("rework-limit", 0, "Rework limit (rounds) for the required criteria")
	flags.verificationLimit = fs.Int("verification-limit", 0, "Verification limit (attempts) for the required criteria")
	flags.reviewWaitLimit = fs.Duration("review-wait-limit", 0, "How long a parked review may wait before a person is asked, for example 2h; unset waits indefinitely")
	return flags
}

// criterionFlags maps --require names to a criterion kind and runner.
var criterionFlags = map[string][2]string{"github-check": {"check", "github"}, "review": {"review", ""}}

// build returns nil when no criterion was required, so the workspace default
// applies.
func (f *criteriaFlags) build() ([]map[string]any, error) {
	if len(f.require) == 0 {
		if len(f.protect) > 0 || *f.reworkLimit > 0 || *f.verificationLimit > 0 || *f.reviewWaitLimit > 0 {
			return nil, fmt.Errorf("--protect and the limit flags need --require")
		}
		return nil, nil
	}
	criteria := make([]map[string]any, 0, len(f.require))
	for _, name := range f.require {
		kind, ok := criterionFlags[strings.TrimSpace(name)]
		if !ok {
			return nil, fmt.Errorf("--require must be github-check or review, got %q", name)
		}
		criteria = append(criteria, f.criterion(kind[0], kind[1]))
	}
	return criteria, nil
}

func (f *criteriaFlags) criterion(kind, runner string) map[string]any {
	criterion := map[string]any{"kind": kind}
	if runner != "" {
		criterion["runner"] = runner
		if len(f.protect) > 0 {
			criterion["protected_paths"] = []string(f.protect)
		}
	}
	if *f.reworkLimit > 0 {
		criterion["rework_limit"] = *f.reworkLimit
	}
	if *f.verificationLimit > 0 {
		criterion["verification_limit"] = *f.verificationLimit
	}
	if kind == "review" && *f.reviewWaitLimit > 0 {
		criterion["review_wait_limit"] = int(f.reviewWaitLimit.Minutes())
	}
	return criterion
}
