package roles

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func TestIsRolesDebug_Config(t *testing.T) {
	viper.Reset()
	viper.Set("roles.debug", true)
	if !IsRolesDebug() {
		t.Error("IsRolesDebug should be true when roles.debug is true")
	}
	viper.Set("roles.debug", false)
	if IsRolesDebug() {
		t.Error("IsRolesDebug should be false when roles.debug is false")
	}
}

func TestDebugOutput_Stderr(t *testing.T) {
	buf := &bytes.Buffer{}
	SetDebugWriter(buf)
	defer SetDebugWriter(os.Stderr)

	viper.Reset()
	viper.Set("roles.debug", true)
	debugf("test message")
	if buf.Len() == 0 {
		t.Error("debugf should write when debug enabled")
	}
	if !strings.Contains(buf.String(), "[ROLES DEBUG]") {
		t.Errorf("output should contain prefix: %q", buf.String())
	}
	if !strings.Contains(buf.String(), "test message") {
		t.Errorf("output should contain message: %q", buf.String())
	}
}

func TestLogLoadedRoles_Deterministic(t *testing.T) {
	buf := &bytes.Buffer{}
	SetDebugWriter(buf)
	viper.Set("roles.debug", true)
	defer func() {
		SetDebugWriter(os.Stderr)
		viper.Reset()
	}()

	resolved := map[string]ResolvedRole{
		"code":  {Name: "code"},
		"base":  {Name: "base"},
		"shell": {Name: "shell"},
	}
	LogLoadedRoles(resolved)
	out := buf.String()
	// Should list roles in deterministic (sorted) order: base, code, shell
	if !strings.Contains(out, "base") || !strings.Contains(out, "code") || !strings.Contains(out, "shell") {
		t.Errorf("output should list all role names: %q", out)
	}
}

func TestLogScore_ContainsScoreAndThreshold(t *testing.T) {
	buf := &bytes.Buffer{}
	SetDebugWriter(buf)
	viper.Set("roles.debug", true)
	defer func() {
		SetDebugWriter(os.Stderr)
		viper.Reset()
	}()

	LogScore("code", 1.3, 0.3, true)
	out := buf.String()
	if !strings.Contains(out, "Score(code)") {
		t.Errorf("output should contain role name: %q", out)
	}
	if !strings.Contains(out, "1.30") || !strings.Contains(out, "0.30") {
		t.Errorf("output should contain score and threshold: %q", out)
	}
	if !strings.Contains(out, "PASS") {
		t.Errorf("output should contain PASS: %q", out)
	}
}
