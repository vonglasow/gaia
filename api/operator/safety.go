package operator

import (
	"strings"
)

// RiskLevel represents the risk level of a tool execution.
type RiskLevel int

const (
	RiskLow RiskLevel = iota
	RiskMedium
	RiskHigh
	RiskCritical
)

func (r RiskLevel) String() string {
	switch r {
	case RiskLow:
		return "low"
	case RiskMedium:
		return "medium"
	case RiskHigh:
		return "high"
	case RiskCritical:
		return "critical"
	default:
		return "unknown"
	}
}

// GuardOptions holds options for the safety guard (denylist, allowlist, confirmation, dry-run).
type GuardOptions struct {
	Denylist          []string
	Allowlist         []string
	ConfirmMediumRisk bool
	DryRun            bool
	Yes               bool
	ConfirmFunc       func(message string) (bool, error)
}

// Allow checks whether a tool call is allowed. It returns (true, "") if allowed,
// (false, reason) if blocked. For run_cmd, the "cmd" arg is checked against denylist/allowlist.
// RiskCritical is always blocked. RiskMedium+ requires confirmation unless Yes or DryRun.
func Allow(tool *Tool, args map[string]string, opts GuardOptions) (allowed bool, reason string) {
	if tool == nil {
		return false, "no tool"
	}
	if tool.RiskLevel == RiskCritical {
		return false, "tool " + tool.Name + " is not allowed (critical risk)"
	}
	cmd := strings.TrimSpace(args["cmd"])
	if cmd == "" && tool.Name == RunCmdName {
		return false, "empty command"
	}
	if tool.Name == RunCmdName {
		cmdLower := strings.ToLower(cmd)
		for _, deny := range opts.Denylist {
			if strings.Contains(cmdLower, strings.ToLower(strings.TrimSpace(deny))) {
				return false, "command blocked by denylist: " + deny
			}
		}
		if len(opts.Allowlist) > 0 {
			allowedByList := false
			for _, allow := range opts.Allowlist {
				if strings.HasPrefix(cmdLower, strings.ToLower(strings.TrimSpace(allow))) ||
					strings.Contains(cmdLower, strings.ToLower(strings.TrimSpace(allow))) {
					allowedByList = true
					break
				}
			}
			if !allowedByList {
				return false, "command not in allowlist"
			}
		}
	}
	if opts.DryRun {
		return true, ""
	}
	if tool.RiskLevel >= RiskMedium && opts.ConfirmMediumRisk && !opts.Yes && opts.ConfirmFunc != nil {
		confirmed, err := opts.ConfirmFunc(formatToolCallForConfirm(tool.Name, args))
		if err != nil {
			return false, "confirmation failed: " + err.Error()
		}
		if !confirmed {
			return false, "user declined"
		}
	}
	return true, ""
}

// formatToolCallForConfirm returns a short description of the tool call for confirmation prompt.
func formatToolCallForConfirm(name string, args map[string]string) string {
	if name == RunCmdName {
		if cmd, ok := args["cmd"]; ok {
			return "Run command: " + cmd
		}
	}
	return name + " with args: " + strings.Join(strings.Fields(strings.TrimSpace(args["cmd"])), " ")
}
