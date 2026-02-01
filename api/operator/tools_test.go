package operator

import (
	"context"
	"testing"
)

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r.Get("x") != nil {
		t.Error("empty registry should return nil")
	}
	if len(r.List()) != 0 {
		t.Errorf("List() = %v, want []", r.List())
	}
}

func TestRegistry_Register_Get(t *testing.T) {
	r := NewRegistry()
	tool := &Tool{Name: "run_cmd", Description: "Run a command", RiskLevel: RiskLow}
	r.Register(tool)
	got := r.Get("run_cmd")
	if got != tool {
		t.Errorf("Get(\"run_cmd\") = %p, want %p", got, tool)
	}
	if r.Get("other") != nil {
		t.Error("Get(\"other\") should be nil")
	}
}

func TestRegistry_Register_overwrites(t *testing.T) {
	r := NewRegistry()
	t1 := &Tool{Name: "a", Description: "first"}
	t2 := &Tool{Name: "a", Description: "second"}
	r.Register(t1)
	r.Register(t2)
	got := r.Get("a")
	if got != t2 {
		t.Errorf("second Register should overwrite; got %p", got)
	}
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()
	r.Register(&Tool{Name: "x"})
	r.Register(&Tool{Name: "y"})
	names := r.List()
	if len(names) != 2 {
		t.Errorf("List() len = %d, want 2", len(names))
	}
	seen := make(map[string]bool)
	for _, n := range names {
		seen[n] = true
	}
	if !seen["x"] || !seen["y"] {
		t.Errorf("List() = %v", names)
	}
}

func TestDefaultToolRegistry(t *testing.T) {
	var runCmd string
	mockRunner := &mockShellRunner{run: func(ctx context.Context, cmd string) (stdout, stderr string, err error) {
		runCmd = cmd
		return "ok", "", nil
	}}
	r := DefaultToolRegistry(mockRunner)
	tool := r.Get(RunCmdName)
	if tool == nil {
		t.Fatal("DefaultToolRegistry should register run_cmd")
	}
	if tool.Name != RunCmdName {
		t.Errorf("tool.Name = %q, want %q", tool.Name, RunCmdName)
	}
	if tool.Exec == nil {
		t.Fatal("tool.Exec should be set")
	}
	_, _, _ = tool.Exec(context.Background(), map[string]string{"cmd": "echo hi"})
	if runCmd != "echo hi" {
		t.Errorf("Exec should call runner with cmd; runCmd = %q", runCmd)
	}
}

func TestDefaultToolRegistry_nilRunner(t *testing.T) {
	r := DefaultToolRegistry(nil)
	tool := r.Get(RunCmdName)
	if tool == nil {
		t.Fatal("registry should still have run_cmd")
	}
	// Exec with nil runner should return "", "", nil (no panic)
	stdout, stderr, err := tool.Exec(context.Background(), map[string]string{"cmd": "x"})
	if stdout != "" || stderr != "" || err != nil {
		t.Errorf("nil runner Exec = %q, %q, %v", stdout, stderr, err)
	}
}

type mockShellRunner struct {
	run func(ctx context.Context, cmd string) (stdout, stderr string, err error)
}

func (m *mockShellRunner) Run(ctx context.Context, cmd string) (stdout, stderr string, err error) {
	if m.run != nil {
		return m.run(ctx, cmd)
	}
	return "", "", nil
}
