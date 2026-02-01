package operator

import (
	"context"
	"errors"
	"strings"
	"testing"
)

func TestNewExecutor(t *testing.T) {
	e := NewExecutor(0)
	if e.MaxOutputBytes != MaxOutputBytes {
		t.Errorf("NewExecutor(0) MaxOutputBytes = %d, want %d", e.MaxOutputBytes, MaxOutputBytes)
	}
	e2 := NewExecutor(100)
	if e2.MaxOutputBytes != 100 {
		t.Errorf("NewExecutor(100) MaxOutputBytes = %d, want 100", e2.MaxOutputBytes)
	}
}

func TestExecutor_Run_nilTool(t *testing.T) {
	e := NewExecutor(100)
	_, _, err := e.Run(context.Background(), nil, map[string]string{})
	if err == nil {
		t.Error("Run(nil tool) should return error")
	}
}

func TestExecutor_Run_nilExec(t *testing.T) {
	e := NewExecutor(100)
	tool := &Tool{Name: "x", Exec: nil}
	_, _, err := e.Run(context.Background(), tool, map[string]string{})
	if err == nil {
		t.Error("Run(tool with nil Exec) should return error")
	}
}

func TestExecutor_Run_truncates(t *testing.T) {
	e := NewExecutor(20)
	tool := &Tool{
		Name: "big",
		Exec: func(ctx context.Context, args map[string]string) (stdout, stderr string, err error) {
			return "012345678901234567890123456789", "", nil
		},
	}
	stdout, _, err := e.Run(context.Background(), tool, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(stdout) <= 25 {
		t.Errorf("expected truncation with (truncated) suffix, got len %d", len(stdout))
	}
	if !strings.HasSuffix(stdout, "(truncated)") {
		t.Errorf("stdout should end with (truncated), got %q", stdout)
	}
}

func TestExecutor_Run_passesThroughError(t *testing.T) {
	e := NewExecutor(100)
	wantErr := errors.New("exec failed")
	tool := &Tool{
		Name: "fail",
		Exec: func(ctx context.Context, args map[string]string) (stdout, stderr string, err error) {
			return "out", "err", wantErr
		},
	}
	_, _, err := e.Run(context.Background(), tool, nil)
	if err != wantErr {
		t.Errorf("Run() err = %v, want %v", err, wantErr)
	}
}

func TestFormatObservation(t *testing.T) {
	got := FormatObservation("hello", "warn", nil)
	if !strings.Contains(got, "stdout:") || !strings.Contains(got, "hello") {
		t.Errorf("FormatObservation with stdout: %q", got)
	}
	if !strings.Contains(got, "stderr:") || !strings.Contains(got, "warn") {
		t.Errorf("FormatObservation with stderr: %q", got)
	}

	got2 := FormatObservation("", "", errors.New("fail"))
	if !strings.Contains(got2, "error:") || !strings.Contains(got2, "fail") {
		t.Errorf("FormatObservation with error: %q", got2)
	}
}
