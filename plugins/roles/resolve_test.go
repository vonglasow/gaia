package roles

import "testing"

func TestResolveSystemPrompt(t *testing.T) {
	role := ResolvedRole{
		Name:         "x",
		SystemPrompt: "base",
		Providers: map[string]ProviderOverride{
			"ollama": {SystemPrompt: "ollama"},
		},
		Models: map[string]ModelOverride{
			"gpt-4.1": {SystemPrompt: "model"},
		},
	}
	if got := ResolveSystemPrompt(role, "ollama", "gpt-4.1"); got != "model" {
		t.Errorf("ResolveSystemPrompt model override = %q", got)
	}
	if got := ResolveSystemPrompt(role, "ollama", ""); got != "ollama" {
		t.Errorf("ResolveSystemPrompt provider override = %q", got)
	}
	if got := ResolveSystemPrompt(role, "", ""); got != "base" {
		t.Errorf("ResolveSystemPrompt base = %q", got)
	}
}

func TestSelectRoleForText(t *testing.T) {
	kw := map[string][]string{
		"code": {"code"},
	}
	res := SelectRoleForText("write code", kw, 1.0, 0.5, "default")
	if res.RoleName != "code" {
		t.Errorf("RoleName = %q, want code", res.RoleName)
	}
}
