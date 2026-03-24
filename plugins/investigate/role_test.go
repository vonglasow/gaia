package investigate

import (
	"os"
	"testing"

	"gaia/plugins/ask"

	"github.com/spf13/viper"
)

func TestApplyInvestigateRole_ExplicitRole(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	dir := t.TempDir()
	viper.Set("roles.directory", dir)
	roleYAML := []byte("name: operator\nsystem_prompt: hello\n")
	if err := os.WriteFile(dir+"/operator.yaml", roleYAML, 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}
	viper.Set("investigate.role", "operator")

	req := &ask.AskRequest{Provider: "ollama", Model: "llama3"}
	if err := applyInvestigateRole(nil, req, "goal"); err != nil {
		t.Fatalf("applyInvestigateRole: %v", err)
	}
	if req.SystemPrompt == "" {
		t.Fatal("expected system prompt")
	}
}

func TestApplyInvestigateRole_AutoSelect(t *testing.T) {
	viper.Reset()
	defer viper.Reset()

	dir := t.TempDir()
	viper.Set("roles.directory", dir)
	roleYAML := []byte("name: code\nsystem_prompt: prompt\n")
	if err := os.WriteFile(dir+"/code.yaml", roleYAML, 0o644); err != nil {
		t.Fatalf("write role: %v", err)
	}
	viper.Set("roles.auto_select", true)
	viper.Set("roles.keywords.code", []string{"code"})

	req := &ask.AskRequest{Provider: "ollama", Model: "llama3"}
	if err := applyInvestigateRole(nil, req, "write code"); err != nil {
		t.Fatalf("applyInvestigateRole: %v", err)
	}
	if req.SystemPrompt == "" {
		t.Fatal("expected system prompt")
	}
}
