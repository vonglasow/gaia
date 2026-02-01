package operator

import (
	"errors"
	"testing"
)

func TestRiskLevel_String(t *testing.T) {
	tests := []struct {
		r    RiskLevel
		want string
	}{
		{RiskLow, "low"},
		{RiskMedium, "medium"},
		{RiskHigh, "high"},
		{RiskCritical, "critical"},
		{RiskLevel(99), "unknown"},
	}
	for _, tt := range tests {
		if got := tt.r.String(); got != tt.want {
			t.Errorf("RiskLevel.String() = %q, want %q", got, tt.want)
		}
	}
}

func TestAllow_nilTool(t *testing.T) {
	allowed, reason := Allow(nil, map[string]string{"cmd": "echo hi"}, GuardOptions{})
	if allowed {
		t.Error("Allow(nil) should not allow")
	}
	if reason == "" {
		t.Error("reason should be non-empty")
	}
}

func TestAllow_criticalRisk(t *testing.T) {
	tool := &Tool{Name: "danger", RiskLevel: RiskCritical}
	allowed, reason := Allow(tool, map[string]string{}, GuardOptions{})
	if allowed {
		t.Error("critical risk tool should be blocked")
	}
	if reason == "" {
		t.Error("reason should be non-empty")
	}
}

func TestAllow_runCmdEmptyCommand(t *testing.T) {
	tool := &Tool{Name: RunCmdName, RiskLevel: RiskLow}
	allowed, reason := Allow(tool, map[string]string{"cmd": "   "}, GuardOptions{})
	if allowed {
		t.Error("empty command should be blocked")
	}
	if reason == "" {
		t.Error("reason should be non-empty")
	}
}

func TestAllow_runCmdDenylist(t *testing.T) {
	tool := &Tool{Name: RunCmdName, RiskLevel: RiskLow}
	opts := GuardOptions{Denylist: []string{"sudo", "rm -rf"}}

	allowed, _ := Allow(tool, map[string]string{"cmd": "df -h"}, opts)
	if !allowed {
		t.Error("df -h should be allowed")
	}

	allowed, reason := Allow(tool, map[string]string{"cmd": "sudo ls"}, opts)
	if allowed {
		t.Error("sudo should be blocked")
	}
	if reason == "" {
		t.Error("reason should be non-empty")
	}

	allowed, _ = Allow(tool, map[string]string{"cmd": "rm -rf /tmp/x"}, opts)
	if allowed {
		t.Error("rm -rf should be blocked")
	}
}

func TestAllow_runCmdAllowlist(t *testing.T) {
	tool := &Tool{Name: RunCmdName, RiskLevel: RiskLow}
	opts := GuardOptions{Allowlist: []string{"df", "du", "find"}}

	allowed, _ := Allow(tool, map[string]string{"cmd": "df -h"}, opts)
	if !allowed {
		t.Error("df -h should be allowed when allowlist has df")
	}

	allowed, reason := Allow(tool, map[string]string{"cmd": "echo hello"}, opts)
	if allowed {
		t.Error("echo should be blocked when not in allowlist")
	}
	if reason == "" {
		t.Error("reason should be non-empty")
	}
}

func TestAllow_mediumRiskNoConfirmWhenYes(t *testing.T) {
	tool := &Tool{Name: RunCmdName, RiskLevel: RiskMedium}
	opts := GuardOptions{ConfirmMediumRisk: true, Yes: true}
	allowed, _ := Allow(tool, map[string]string{"cmd": "touch /tmp/x"}, opts)
	if !allowed {
		t.Error("with Yes, medium risk should be allowed without confirm")
	}
}

func TestAllow_mediumRiskConfirmDeclined(t *testing.T) {
	tool := &Tool{Name: RunCmdName, RiskLevel: RiskMedium}
	opts := GuardOptions{
		ConfirmMediumRisk: true,
		Yes:               false,
		ConfirmFunc: func(message string) (bool, error) {
			return false, nil
		},
	}
	allowed, reason := Allow(tool, map[string]string{"cmd": "touch /tmp/x"}, opts)
	if allowed {
		t.Error("when user declines, should not allow")
	}
	if reason != "user declined" {
		t.Errorf("reason = %q, want %q", reason, "user declined")
	}
}

func TestAllow_mediumRiskConfirmError(t *testing.T) {
	tool := &Tool{Name: RunCmdName, RiskLevel: RiskMedium}
	opts := GuardOptions{
		ConfirmMediumRisk: true,
		Yes:               false,
		ConfirmFunc: func(message string) (bool, error) {
			return false, errors.New("read error")
		},
	}
	allowed, reason := Allow(tool, map[string]string{"cmd": "touch /tmp/x"}, opts)
	if allowed {
		t.Error("when confirm returns error, should not allow")
	}
	if reason == "" {
		t.Error("reason should mention confirmation failed")
	}
}

func TestAllow_dryRun(t *testing.T) {
	tool := &Tool{Name: RunCmdName, RiskLevel: RiskMedium}
	opts := GuardOptions{DryRun: true, ConfirmMediumRisk: true, Yes: false}
	allowed, _ := Allow(tool, map[string]string{"cmd": "rm -rf /"}, opts)
	if !allowed {
		t.Error("dry-run should allow (no actual execution)")
	}
}

func Test_formatToolCallForConfirm(t *testing.T) {
	got := formatToolCallForConfirm(RunCmdName, map[string]string{"cmd": "df -h"})
	if got != "Run command: df -h" {
		t.Errorf("formatToolCallForConfirm = %q", got)
	}
}
