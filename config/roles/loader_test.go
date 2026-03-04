package roles

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadRoles(t *testing.T) {
	dir := t.TempDir()
	// Create a minimal role file
	yaml := `name: test
description: Test role
priority: 10
system_prompt: "You are a test."
matching:
  signals:
    - type: keyword
      values: ["test"]
      weight: 1.0
`
	if err := os.WriteFile(filepath.Join(dir, "test.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}

	roleList, err := LoadRoles(dir)
	if err != nil {
		t.Fatalf("LoadRoles: %v", err)
	}
	if len(roleList) != 1 {
		t.Fatalf("expected 1 role, got %d", len(roleList))
	}
	if roleList[0].Name != "test" {
		t.Errorf("role name = %q, want test", roleList[0].Name)
	}
	if roleList[0].SystemPrompt != "You are a test." {
		t.Errorf("system_prompt = %q", roleList[0].SystemPrompt)
	}
	if len(roleList[0].Matching.Signals) != 1 {
		t.Errorf("expected 1 signal, got %d", len(roleList[0].Matching.Signals))
	}
}

func TestLoadRoles_WithSignalsImport(t *testing.T) {
	dir := t.TempDir()
	signalsDir := filepath.Join(dir, "_signals")
	if err := os.MkdirAll(signalsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	groupYaml := `group: common.foo
type: keyword
values:
  - alpha
  - beta
`
	if err := os.WriteFile(filepath.Join(signalsDir, "foo.yaml"), []byte(groupYaml), 0o600); err != nil {
		t.Fatal(err)
	}
	roleYaml := `name: bar
system_prompt: "Bar."
matching:
  imports:
    - common.foo
  signals: []
`
	if err := os.WriteFile(filepath.Join(dir, "bar.yaml"), []byte(roleYaml), 0o600); err != nil {
		t.Fatal(err)
	}

	roleList, err := LoadRoles(dir)
	if err != nil {
		t.Fatalf("LoadRoles: %v", err)
	}
	if len(roleList) != 1 {
		t.Fatalf("expected 1 role, got %d", len(roleList))
	}
	// Resolved signals should include the imported group (one Signal with multiple values)
	if len(roleList[0].Matching.Signals) != 1 {
		t.Errorf("expected 1 signal group from import, got %d", len(roleList[0].Matching.Signals))
	}
	if len(roleList[0].Matching.Signals[0].Values) != 2 {
		t.Errorf("expected 2 values in imported signal, got %d", len(roleList[0].Matching.Signals[0].Values))
	}
}

func TestLoadRoles_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	roleList, err := LoadRoles(dir)
	if err != nil {
		t.Fatalf("LoadRoles: %v", err)
	}
	if len(roleList) != 0 {
		t.Errorf("expected 0 roles, got %d", len(roleList))
	}
}

func TestLoadRoles_InvalidDir(t *testing.T) {
	_, err := LoadRoles("/nonexistent")
	if err == nil {
		t.Fatal("expected error for nonexistent dir")
	}
}

func TestLoadRoles_UnknownImport(t *testing.T) {
	dir := t.TempDir()
	yaml := `name: x
system_prompt: "X"
matching:
  imports:
    - unknown.group
  signals: []
`
	if err := os.WriteFile(filepath.Join(dir, "x.yaml"), []byte(yaml), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadRoles(dir)
	if err == nil {
		t.Fatal("expected error for unknown import")
	}
}

func TestLoadRoles_WithExtends(t *testing.T) {
	dir := t.TempDir()
	baseYaml := `name: base
description: Base role
system_prompt: "You are base."
matching:
  signals:
    - type: keyword
      values: [base]
      weight: 1.0
`
	childYaml := `name: child
extends: [base]
system_prompt: "You are child."
`
	if err := os.WriteFile(filepath.Join(dir, "base.yaml"), []byte(baseYaml), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "child.yaml"), []byte(childYaml), 0o600); err != nil {
		t.Fatal(err)
	}
	roleList, err := LoadRoles(dir)
	if err != nil {
		t.Fatalf("LoadRoles: %v", err)
	}
	if len(roleList) != 2 {
		t.Fatalf("expected 2 roles, got %d", len(roleList))
	}
	var child ResolvedRole
	for _, r := range roleList {
		if r.Name == "child" {
			child = r
			break
		}
	}
	if child.Name != "child" {
		t.Fatal("child role not found")
	}
	if !strings.Contains(child.SystemPrompt, "You are base.") || !strings.Contains(child.SystemPrompt, "You are child.") {
		t.Errorf("child should inherit base prompt: %q", child.SystemPrompt)
	}
	if len(child.Matching.Signals) == 0 {
		t.Error("child should inherit base signals")
	}
}
