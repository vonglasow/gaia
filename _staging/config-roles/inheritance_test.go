package roles

import (
	"testing"
)

func TestResolveInheritance_SingleParent(t *testing.T) {
	roles := map[string]Role{
		"base": {
			Name:         "base",
			SystemPrompt: "Base prompt.",
			Matching:     MatchingConfig{Signals: []Signal{{Type: "keyword", Values: []string{"base"}, Weight: 1.0}}},
		},
		"child": {
			Name:         "child",
			Extends:      []string{"base"},
			SystemPrompt: "Child prompt.",
		},
	}
	resolved, err := ResolveInheritance(roles)
	if err != nil {
		t.Fatalf("ResolveInheritance: %v", err)
	}
	if len(resolved) != 2 {
		t.Fatalf("expected 2 resolved roles, got %d", len(resolved))
	}
	c := resolved["child"]
	// Parent first, then separator, then child
	if c.SystemPrompt != "Base prompt."+systemPromptSeparator+"Child prompt." {
		t.Errorf("child system prompt = %q", c.SystemPrompt)
	}
	if len(c.Matching.Signals) != 1 {
		t.Errorf("expected 1 signal from base, got %d", len(c.Matching.Signals))
	}
}

func TestResolveInheritance_MultipleParents(t *testing.T) {
	roles := map[string]Role{
		"a": {Name: "a", SystemPrompt: "A."},
		"b": {Name: "b", SystemPrompt: "B."},
		"c": {Name: "c", Extends: []string{"a", "b"}, SystemPrompt: "C."},
	}
	resolved, err := ResolveInheritance(roles)
	if err != nil {
		t.Fatalf("ResolveInheritance: %v", err)
	}
	got := resolved["c"].SystemPrompt
	// Order: a, then b, then c (parents in extends order, then child)
	want := "A." + systemPromptSeparator + "B." + systemPromptSeparator + "C."
	if got != want {
		t.Errorf("system prompt = %q, want %q", got, want)
	}
}

func TestResolveInheritance_DeepChain(t *testing.T) {
	roles := map[string]Role{
		"grandparent": {Name: "grandparent", SystemPrompt: "Grandparent."},
		"parent":      {Name: "parent", Extends: []string{"grandparent"}, SystemPrompt: "Parent."},
		"child":       {Name: "child", Extends: []string{"parent"}, SystemPrompt: "Child."},
	}
	resolved, err := ResolveInheritance(roles)
	if err != nil {
		t.Fatalf("ResolveInheritance: %v", err)
	}
	got := resolved["child"].SystemPrompt
	want := "Grandparent." + systemPromptSeparator + "Parent." + systemPromptSeparator + "Child."
	if got != want {
		t.Errorf("system prompt = %q, want %q", got, want)
	}
}

func TestResolveInheritance_ChildOverridesPriority(t *testing.T) {
	pri10 := 10
	pri5 := 5
	roles := map[string]Role{
		"base":  {Name: "base", Priority: &pri10},
		"child": {Name: "child", Extends: []string{"base"}, Priority: &pri5},
	}
	resolved, err := ResolveInheritance(roles)
	if err != nil {
		t.Fatalf("ResolveInheritance: %v", err)
	}
	if resolved["child"].Priority != 5 {
		t.Errorf("child priority = %d, want 5", resolved["child"].Priority)
	}
}

func TestResolveInheritance_ChildOverridesSystemPrompt(t *testing.T) {
	roles := map[string]Role{
		"base":  {Name: "base", SystemPrompt: "Base."},
		"child": {Name: "child", Extends: []string{"base"}, SystemPrompt: "Only child."},
	}
	resolved, err := ResolveInheritance(roles)
	if err != nil {
		t.Fatalf("ResolveInheritance: %v", err)
	}
	got := resolved["child"].SystemPrompt
	want := "Base." + systemPromptSeparator + "Only child."
	if got != want {
		t.Errorf("system prompt = %q, want %q", got, want)
	}
}

func TestResolveInheritance_CircularError(t *testing.T) {
	roles := map[string]Role{
		"a": {Name: "a", Extends: []string{"c"}},
		"b": {Name: "b", Extends: []string{"a"}},
		"c": {Name: "c", Extends: []string{"b"}},
	}
	_, err := ResolveInheritance(roles)
	if err == nil {
		t.Fatal("expected error for circular inheritance")
	}
	if err != nil && err.Error() == "" {
		t.Errorf("error should mention circular/circle: %v", err)
	}
}

func TestResolveInheritance_MissingParentError(t *testing.T) {
	roles := map[string]Role{
		"child": {Name: "child", Extends: []string{"missing"}},
	}
	_, err := ResolveInheritance(roles)
	if err == nil {
		t.Fatal("expected error for missing parent")
	}
}
