package roles

import (
	"testing"
	"testing/fstest"
)

func TestLoadRolesFromFS(t *testing.T) {
	fsys := fstest.MapFS{
		"roles/default.yaml": {Data: []byte("system_prompt: hello\n")},
		"roles/skip.txt":     {Data: []byte("ignore")},
	}
	roles, err := LoadRolesFromFS(fsys, "roles")
	if err != nil {
		t.Fatalf("LoadRolesFromFS: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(roles))
	}
	if roles[0].Name != "default" {
		t.Errorf("role name = %q", roles[0].Name)
	}
}

func TestDefaultRolesDir(t *testing.T) {
	dir, err := DefaultRolesDir()
	if err != nil {
		t.Fatalf("DefaultRolesDir: %v", err)
	}
	if dir == "" {
		t.Fatal("DefaultRolesDir returned empty")
	}
}

func TestValidateRoleRequiresPrompt(t *testing.T) {
	fsys := fstest.MapFS{
		"roles/bad.yaml": {Data: []byte("name: bad\n")},
	}
	_, err := LoadRolesFromFS(fsys, "roles")
	if err == nil {
		t.Fatal("expected error for missing prompt")
	}
}
