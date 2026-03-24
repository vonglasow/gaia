package roles

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

// ScoreRule defines scoring keywords and weights.
type ScoreRule struct {
	Keywords []string
	Weight   float64
}

// ScoringConfig maps role name to score rules.
type ScoringConfig map[string]ScoreRule

// ScoreText returns a score for each role based on keyword matches.
func ScoreText(text string, rules ScoringConfig) map[string]float64 {
	text = strings.ToLower(text)
	scores := make(map[string]float64, len(rules))
	for role, rule := range rules {
		score := 0.0
		for _, kw := range rule.Keywords {
			kw = strings.TrimSpace(kw)
			if kw == "" {
				continue
			}
			if containsWord(text, strings.ToLower(kw)) {
				score += rule.Weight
			}
		}
		scores[role] = score
	}
	return scores
}

// SelectRole chooses the best role based on scores and threshold.
func SelectRole(scores map[string]float64, threshold float64, defaultRole string) SelectionResult {
	type pair struct {
		name  string
		score float64
	}
	var pairs []pair
	for name, score := range scores {
		pairs = append(pairs, pair{name: name, score: score})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score == pairs[j].score {
			return pairs[i].name < pairs[j].name
		}
		return pairs[i].score > pairs[j].score
	})

	result := SelectionResult{
		Threshold: threshold,
		AllScores: scores,
	}
	for _, p := range pairs {
		result.SortedRoles = append(result.SortedRoles, p.name)
		if p.score >= threshold && result.RoleName == "" {
			result.RoleName = p.name
			result.Score = p.score
			result.Matched = true
		}
	}
	if result.RoleName == "" {
		result.RoleName = defaultRole
		result.Score = scores[defaultRole]
		result.Matched = false
		result.Reason = "no role met threshold"
	}
	if result.RoleName == "" {
		result.Reason = "no roles available"
	}
	return result
}

var wordSplit = regexp.MustCompile(`\W+`)

func containsWord(text, word string) bool {
	if word == "" {
		return false
	}
	parts := wordSplit.Split(text, -1)
	for _, p := range parts {
		if p == word {
			return true
		}
	}
	return strings.Contains(text, word)
}

// BuildScoringFromConfig converts simple keyword lists into scoring rules.
func BuildScoringFromConfig(keywords map[string][]string, weight float64) ScoringConfig {
	rules := make(ScoringConfig, len(keywords))
	for role, list := range keywords {
		rules[role] = ScoreRule{Keywords: list, Weight: weight}
	}
	return rules
}

func ValidateScoringConfig(scoring ScoringConfig) error {
	for role, rule := range scoring {
		if len(rule.Keywords) == 0 {
			return fmt.Errorf("role %q has empty keyword list", role)
		}
		if rule.Weight == 0 {
			return fmt.Errorf("role %q has zero weight", role)
		}
	}
	return nil
}
