package tools

import "testing"

func TestBuildCommandArgs(t *testing.T) {
	name, args, err := buildCommandArgs("git checkout -b {response}", map[string]string{
		"{response}": "feat-1",
		"{output}":   "feat-1",
		"{file}":     "/tmp/x",
	})
	if err != nil {
		t.Fatalf("buildCommandArgs: %v", err)
	}
	if name != "git" {
		t.Fatalf("name = %q", name)
	}
	if len(args) < 1 || args[len(args)-1] != "feat-1" {
		t.Fatalf("args = %v", args)
	}
}

func TestRequiresShell(t *testing.T) {
	if !requiresShell("echo hi | wc -l") {
		t.Fatal("expected requiresShell for pipe")
	}
	if requiresShell("git status") {
		t.Fatal("did not expect requiresShell")
	}
}
