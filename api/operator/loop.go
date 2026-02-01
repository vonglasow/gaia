package operator

import (
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
)

// ErrMaxStepsReached is returned when the operator exits after max_steps without an answer.
var ErrMaxStepsReached = errors.New("max steps reached")

// RunOptions holds options for the operator run (max steps, dry-run, yes, debug, guard, model).
type RunOptions struct {
	MaxSteps          int
	DryRun            bool
	Yes               bool
	Debug             bool
	Model             string
	Denylist          []string
	Allowlist         []string
	ConfirmMediumRisk bool
	ConfirmFunc       func(message string) (bool, error)
	ShellRunner       ShellRunner
	MaxOutputBytes    int
	MaxParseFailures  int
}

// Run runs the operator loop: planner → guard → executor → observer until answer or max_steps.
func Run(ctx context.Context, goal string, opts RunOptions) (finalAnswer string, err error) {
	goal = strings.TrimSpace(goal)
	if goal == "" {
		return "", fmt.Errorf("goal cannot be empty")
	}
	if opts.MaxSteps <= 0 {
		opts.MaxSteps = 10
	}
	if opts.MaxParseFailures <= 0 {
		opts.MaxParseFailures = 2
	}

	state := &State{Goal: goal, Steps: nil}
	registry := DefaultToolRegistry(opts.ShellRunner)
	planner := &Planner{Model: opts.Model, SendReq: nil}
	executor := NewExecutor(opts.MaxOutputBytes)
	guardOpts := GuardOptions{
		Denylist:          opts.Denylist,
		Allowlist:         opts.Allowlist,
		ConfirmMediumRisk: opts.ConfirmMediumRisk,
		DryRun:            opts.DryRun,
		Yes:               opts.Yes,
		ConfirmFunc:       opts.ConfirmFunc,
	}

	parseFailures := 0
	for step := 0; step < opts.MaxSteps; step++ {
		decision, raw, parseErr := planner.Decide(ctx, state, registry)
		if parseErr != nil {
			parseFailures++
			state.AppendObservation("error: Invalid response: " + parseErr.Error() + ". Respond with valid JSON only.")
			if opts.Debug {
				fmt.Fprintf(os.Stderr, "[DEBUG] parse error: %v\n", parseErr)
			}
			if parseFailures >= opts.MaxParseFailures {
				return state.LastAnswerOrPartial(), fmt.Errorf("repeated parse failures: %w", parseErr)
			}
			continue
		}
		parseFailures = 0

		state.AppendDecision(raw)

		if opts.Debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] decision: action=%s", decision.Action)
			if decision.Action == "tool" {
				fmt.Fprintf(os.Stderr, " name=%s args=%v", decision.Name, decision.Args)
			}
			if decision.Reasoning != "" {
				fmt.Fprintf(os.Stderr, " reasoning=%q", decision.Reasoning)
			}
			fmt.Fprintf(os.Stderr, "\n")
		}

		if decision.Action == "answer" {
			return strings.TrimSpace(decision.Content), nil
		}

		tool := registry.Get(decision.Name)
		if tool == nil {
			state.AppendObservation("error: Unknown tool: " + decision.Name)
			if opts.Debug {
				fmt.Fprintf(os.Stderr, "[DEBUG] observation: unknown tool %s\n", decision.Name)
			}
			continue
		}

		allowed, reason := Allow(tool, decision.Args, guardOpts)
		if !allowed {
			state.AppendObservation("blocked: " + reason)
			if opts.Debug {
				fmt.Fprintf(os.Stderr, "[DEBUG] observation: blocked %s\n", reason)
			}
			continue
		}

		if opts.DryRun {
			obs := "dry_run: Would run: " + decision.Name
			if cmd, ok := decision.Args["cmd"]; ok {
				obs += " " + cmd
			}
			state.AppendObservation(obs)
			if opts.Debug {
				fmt.Fprintf(os.Stderr, "[DEBUG] observation: %s\n", obs)
			}
			continue
		}

		stdout, stderr, execErr := executor.Run(ctx, tool, decision.Args)
		obs := FormatObservation(stdout, stderr, execErr)
		state.AppendObservation(obs)
		if opts.Debug {
			trunc := obs
			if len(trunc) > 200 {
				trunc = trunc[:200] + "..."
			}
			fmt.Fprintf(os.Stderr, "[DEBUG] observation: %s\n", trunc)
		}
	}

	return state.LastAnswerOrPartial(), ErrMaxStepsReached
}
