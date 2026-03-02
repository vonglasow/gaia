package roles

import (
	"fmt"
	"regexp"
	"strings"
)

// ScoreRole returns a score for the role against the input. Higher = better match.
// Keyword matches add weight per occurrence; regex matches add weight per match;
// negative signals subtract. Priority is added as a small factor. Score is
// normalized by input length to avoid bias on long prompts.
func ScoreRole(role Role, input string) float64 {
	inputLower := strings.ToLower(strings.TrimSpace(input))
	inputLen := len(strings.Fields(inputLower))
	if inputLen == 0 {
		inputLen = 1
	}

	score := 0.0

	for _, s := range role.Matching.Signals {
		w := s.Weight
		if w == 0 {
			w = 1.0
		}
		switch strings.ToLower(s.Type) {
		case "keyword":
			for _, v := range s.Values {
				score += float64(strings.Count(inputLower, strings.ToLower(v))) * w
			}
		case "regex":
			for _, pat := range s.Values {
				re, err := regexp.Compile("(?i)" + pat)
				if err != nil {
					continue
				}
				matches := re.FindAllString(inputLower, -1)
				score += float64(len(matches)) * w
			}
		}
	}

	for _, s := range role.Matching.NegativeSignals {
		w := s.Weight
		if w == 0 {
			w = 1.0
		}
		switch strings.ToLower(s.Type) {
		case "keyword":
			for _, v := range s.Values {
				score -= float64(strings.Count(inputLower, strings.ToLower(v))) * w
			}
		case "regex":
			for _, pat := range s.Values {
				re, err := regexp.Compile("(?i)" + pat)
				if err != nil {
					continue
				}
				matches := re.FindAllString(inputLower, -1)
				score -= float64(len(matches)) * w
			}
		}
	}

	// Priority factor: higher priority adds a small constant so ties go to higher priority
	const priorityScale = 0.01
	score += float64(role.Priority) * priorityScale

	// Normalize by input length to avoid long prompts dominating
	normalizer := 1.0 + float64(inputLen)/50.0
	score = score / normalizer

	return score
}

// SelectRole chooses the best role for the input: only enabled roles are scored,
// threshold (role-specific or global) is applied, highest score wins, with
// fallback to default_role if none qualify.
func SelectRole(input string, roles []Role, cfg RolesConfig) Role {
	var defaultRole Role
	for i := range roles {
		if roles[i].Name == cfg.DefaultRole {
			defaultRole = roles[i]
			break
		}
	}
	if defaultRole.Name == "" {
		defaultRole = Role{Name: cfg.DefaultRole}
	}

	threshold := cfg.MinThreshold
	if threshold <= 0 {
		threshold = 0.3
	}

	var best Role
	bestScore := -1e9
	for _, r := range roles {
		if r.Enabled != nil && !*r.Enabled {
			continue
		}
		th := threshold
		if r.Matching.Threshold > 0 {
			th = r.Matching.Threshold
		}
		s := ScoreRole(r, input)
		if s >= th && s > bestScore {
			bestScore = s
			best = r
		}
	}
	if best.Name != "" {
		return best
	}
	return defaultRole
}

// SystemPromptForRole returns the system prompt for the role, with optional
// template args (e.g. SHELL, GOOS). If the role has no SystemPrompt, returns empty.
func SystemPromptForRole(r Role, args ...string) string {
	if r.SystemPrompt == "" {
		return ""
	}
	if len(args) >= 2 {
		return fmt.Sprintf(r.SystemPrompt, args[0], args[1])
	}
	if len(args) == 1 {
		return fmt.Sprintf(r.SystemPrompt, args[0])
	}
	return r.SystemPrompt
}
