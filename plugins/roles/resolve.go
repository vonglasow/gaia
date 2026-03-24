package roles

import "strings"

// ResolveSystemPrompt returns the best system prompt for a role, considering model/provider overrides.
func ResolveSystemPrompt(role ResolvedRole, provider, model string) string {
	model = strings.TrimSpace(model)
	provider = strings.TrimSpace(provider)
	if model != "" {
		if override, ok := role.Models[model]; ok && strings.TrimSpace(override.SystemPrompt) != "" {
			return override.SystemPrompt
		}
	}
	if provider != "" {
		if override, ok := role.Providers[provider]; ok && strings.TrimSpace(override.SystemPrompt) != "" {
			return override.SystemPrompt
		}
	}
	return role.SystemPrompt
}

// SelectRoleForText scores text and selects a role using keyword rules.
func SelectRoleForText(text string, keywords map[string][]string, weight float64, threshold float64, defaultRole string) SelectionResult {
	rules := BuildScoringFromConfig(keywords, weight)
	scores := ScoreText(text, rules)
	return SelectRole(scores, threshold, defaultRole)
}
