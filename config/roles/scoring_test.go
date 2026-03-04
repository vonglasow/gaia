package roles

import (
	"testing"
)

func TestScoreRole_Keyword(t *testing.T) {
	r := ResolvedRole{
		Name: "code", Enabled: true, Weight: 1.0,
		Matching: MatchingConfig{
			Signals: []Signal{
				{Type: "keyword", Values: []string{"function", "python"}, Weight: 1.0},
			},
		},
	}
	score := ScoreRole(r, "write a python function")
	if score <= 0 {
		t.Errorf("expected positive score, got %.2f", score)
	}
	scoreNone := ScoreRole(r, "hello world")
	if scoreNone >= score {
		t.Errorf("expected lower score for non-matching input")
	}
}

func TestScoreRole_NegativeSignals(t *testing.T) {
	r := ResolvedRole{
		Name: "code", Enabled: true, Weight: 1.0,
		Matching: MatchingConfig{
			Signals: []Signal{
				{Type: "keyword", Values: []string{"code"}, Weight: 1.0},
			},
			NegativeSignals: []Signal{
				{Type: "keyword", Values: []string{"recipe", "cooking"}, Weight: 2.0},
			},
		},
	}
	scoreCode := ScoreRole(r, "write code")
	scoreRecipe := ScoreRole(r, "recipe and cooking")
	if scoreRecipe >= scoreCode {
		t.Errorf("negative signals should reduce score: code=%.2f recipe=%.2f", scoreCode, scoreRecipe)
	}
}

func TestSelectRole_Threshold(t *testing.T) {
	roles := []ResolvedRole{
		{Name: "default", Enabled: true, Weight: 1.0, Matching: MatchingConfig{Signals: []Signal{}}},
		{Name: "shell", Enabled: true, Priority: 10, Weight: 1.0, Matching: MatchingConfig{
			Signals: []Signal{{Type: "keyword", Values: []string{"run", "command"}, Weight: 1.0}},
		}},
	}
	cfg := RolesConfig{DefaultRole: "default", MinThreshold: 0.3}

	selected := SelectRole("run ls command", roles, cfg)
	if selected.Name != "shell" {
		t.Errorf("SelectRole = %q, want shell", selected.Name)
	}

	selectedDefault := SelectRole("hello world", roles, cfg)
	if selectedDefault.Name != "default" {
		t.Errorf("SelectRole(no match) = %q, want default", selectedDefault.Name)
	}
}

func TestSelectRole_DisabledRole(t *testing.T) {
	roles := []ResolvedRole{
		{Name: "default", Enabled: true, Weight: 1.0, Matching: MatchingConfig{}},
		{Name: "shell", Enabled: false, Priority: 10, Weight: 1.0, Matching: MatchingConfig{
			Signals: []Signal{{Type: "keyword", Values: []string{"run"}, Weight: 1.0}},
		}},
	}
	cfg := RolesConfig{DefaultRole: "default", MinThreshold: 0.1}
	selected := SelectRole("run command", roles, cfg)
	if selected.Name != "default" {
		t.Errorf("disabled role should be skipped: got %q", selected.Name)
	}
}

func TestSelectRole_FallbackToDefault(t *testing.T) {
	roles := []ResolvedRole{
		{Name: "default", Enabled: true, Weight: 1.0, Matching: MatchingConfig{}},
		{Name: "code", Enabled: true, Priority: 10, Weight: 1.0, Matching: MatchingConfig{
			Signals: []Signal{{Type: "keyword", Values: []string{"xyz"}, Weight: 1.0}},
		}},
	}
	cfg := RolesConfig{DefaultRole: "default", MinThreshold: 0.3}
	selected := SelectRole("hello world", roles, cfg)
	if selected.Name != "default" {
		t.Errorf("SelectRole(no match) = %q, want default", selected.Name)
	}
}

func TestSystemPromptForRole(t *testing.T) {
	r := ResolvedRole{SystemPrompt: "Use %s on %s."}
	out := SystemPromptForRole(r, "bash", "linux")
	if out != "Use bash on linux." {
		t.Errorf("SystemPromptForRole = %q", out)
	}
	outNone := SystemPromptForRole(r)
	if outNone != "Use %s on %s." {
		t.Errorf("no args should return template as-is: %q", outNone)
	}
}
