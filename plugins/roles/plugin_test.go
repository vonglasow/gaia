package roles

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
)

func TestLoadKeywordConfig(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	viper.Set("roles.keywords.code", []string{"code", "bug"})
	viper.Set("roles.keywords.shell", []string{"shell"})

	kw := LoadKeywordConfig()
	if len(kw) != 2 {
		t.Fatalf("expected 2 keyword lists, got %d", len(kw))
	}
	if len(kw["code"]) != 2 {
		t.Errorf("code keywords = %v", kw["code"])
	}
}

func TestLoadRolesWithDefaults_UsesConfigDir(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	dir := t.TempDir()
	viper.Set("roles.directory", dir)
	roles, err := LoadRolesWithDefaults()
	if err != nil {
		t.Fatalf("loadRolesWithDefaults: %v", err)
	}
	if len(roles) != 0 {
		t.Fatalf("expected empty roles, got %d", len(roles))
	}

	roleFile := filepath.Join(dir, "default.yaml")
	if err := os.WriteFile(roleFile, []byte("system_prompt: hello\n"), 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}
	roles, err = LoadRolesWithDefaults()
	if err != nil {
		t.Fatalf("loadRolesWithDefaults: %v", err)
	}
	if len(roles) != 1 {
		t.Fatalf("expected 1 role, got %d", len(roles))
	}
}
