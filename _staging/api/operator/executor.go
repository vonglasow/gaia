package operator

import (
	"context"
	"fmt"
	"strings"
)

// MaxOutputBytes is the default maximum length for combined stdout+stderr in an observation.
const MaxOutputBytes = 4096

// Executor runs tools and truncates output.
type Executor struct {
	MaxOutputBytes int
}

// NewExecutor returns an executor with default max output size.
func NewExecutor(maxBytes int) *Executor {
	if maxBytes <= 0 {
		maxBytes = MaxOutputBytes
	}
	return &Executor{MaxOutputBytes: maxBytes}
}

// Run executes the tool with the given args and returns stdout, stderr, and error.
// Output is truncated to MaxOutputBytes with a "(truncated)" suffix if needed.
func (e *Executor) Run(ctx context.Context, tool *Tool, args map[string]string) (stdout, stderr string, err error) {
	if tool == nil || tool.Exec == nil {
		return "", "", fmt.Errorf("nil tool or exec")
	}
	stdout, stderr, err = tool.Exec(ctx, args)
	stdout = e.truncate(stdout)
	stderr = e.truncate(stderr)
	return stdout, stderr, err
}

func (e *Executor) truncate(s string) string {
	max := e.MaxOutputBytes
	if max <= 0 {
		max = MaxOutputBytes
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n(truncated)"
}

// FormatObservation returns a single string suitable for appending to state (user message).
func FormatObservation(stdout, stderr string, execErr error) string {
	var b strings.Builder
	if stdout != "" {
		b.WriteString("stdout:\n")
		b.WriteString(stdout)
		b.WriteString("\n")
	}
	if stderr != "" {
		b.WriteString("stderr:\n")
		b.WriteString(stderr)
		b.WriteString("\n")
	}
	if execErr != nil {
		b.WriteString("error: ")
		b.WriteString(execErr.Error())
	}
	return strings.TrimSpace(b.String())
}
