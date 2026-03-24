package roles

import "testing"

func TestResolveInheritance(t *testing.T) {
	roles := []Role{
		{Name: "base", SystemPrompt: "base", Providers: map[string]ProviderOverride{"ollama": {SystemPrompt: "base-ollama"}}},
		{Name: "child", SystemPrompt: "child", Extends: []string{"base"}, Providers: map[string]ProviderOverride{"ollama": {SystemPrompt: "child-ollama"}}},
	}
	resolved, err := ResolveInheritance(roles)
	if err != nil {
		t.Fatalf("ResolveInheritance: %v", err)
	}
	if resolved["child"].SystemPrompt == "" || resolved["child"].SystemPrompt == "child" {
		t.Errorf("expected inherited prompt, got %q", resolved["child"].SystemPrompt)
	}
	if resolved["child"].Providers["ollama"].SystemPrompt != "child-ollama" {
		t.Errorf("provider override not merged, got %q", resolved["child"].Providers["ollama"].SystemPrompt)
	}
}

func TestResolveInheritanceCycle(t *testing.T) {
	roles := []Role{
		{Name: "a", SystemPrompt: "a", Extends: []string{"b"}},
		{Name: "b", SystemPrompt: "b", Extends: []string{"a"}},
	}
	_, err := ResolveInheritance(roles)
	if err == nil {
		t.Fatal("expected cycle error")
	}
}
