package api

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

// DetectionResult contains the result of role detection
type DetectionResult struct {
	Role   string  `json:"role"`
	Method string  `json:"method"` // "heuristic" | "llm" | "explicit" | "default"
	Score  float64 `json:"score,omitempty"`
	Reason string  `json:"reason,omitempty"`
}

// getRoleKeywords retrieves keywords for a role from configuration
// Returns empty slice if no keywords are configured for the role
func getRoleKeywords(role string) []string {
	key := fmt.Sprintf("auto_role.keywords.%s", role)
	if !viper.IsSet(key) {
		return []string{}
	}

	// Try to get as string slice
	keywords := viper.GetStringSlice(key)
	if len(keywords) > 0 {
		return keywords
	}

	// Fallback: try to get as interface slice and convert
	if raw := viper.Get(key); raw != nil {
		if slice, ok := raw.([]interface{}); ok {
			result := make([]string, 0, len(slice))
			for _, item := range slice {
				if str, ok := item.(string); ok {
					result = append(result, str)
				}
			}
			return result
		}
	}

	return []string{}
}

// codePatterns are regex patterns that indicate code presence
var codePatterns = []*regexp.Regexp{
	regexp.MustCompile(`\b(def|class|function|const|let|var|import|from|return|if|else|for|while|try|catch)\b`),
	regexp.MustCompile(`[{}();]`),
	regexp.MustCompile(`\b(function|=>|->|::)\b`),
	regexp.MustCompile(`\b(public|private|protected|static|final|abstract)\b`),
}

// detectRoleHeuristic performs fast local heuristic-based role detection
func detectRoleHeuristic(message string, availableRoles []string) (string, float64, string) {
	messageLower := strings.ToLower(strings.TrimSpace(message))
	messageWords := strings.Fields(messageLower)

	// For long messages (like git diff + user request), focus on the end where the user request is
	// Take last 50 words or last 500 characters, whichever is smaller
	requestPortion := messageLower
	if len(messageWords) > 50 {
		// Take last 50 words
		requestWords := messageWords[len(messageWords)-50:]
		requestPortion = strings.Join(requestWords, " ")
	} else if len(messageLower) > 500 {
		// Take last 500 characters
		if len(messageLower) > 500 {
			requestPortion = messageLower[len(messageLower)-500:]
		}
	}

	// Score each role based on keyword matches
	scores := make(map[string]float64)
	for _, role := range availableRoles {
		// Get keywords for this role from configuration
		keywords := getRoleKeywords(role)
		if len(keywords) == 0 {
			continue // Skip roles without keywords configured
		}

		// Count keyword matches with weighted scoring
		matches := 0
		phraseMatches := 0 // Multi-word phrases get higher weight

		// For "describe" role, give extra weight if question words appear at the start
		if role == "describe" && len(messageWords) > 0 {
			firstWord := messageWords[0]
			if firstWord == "what" || firstWord == "explain" || firstWord == "describe" ||
				firstWord == "tell" || firstWord == "how" {
				matches += 3 // Boost for question words at start
			}
		}

		// First pass: check for multi-word phrases (higher priority)
		// Check exact phrase matches first in the request portion (where user intent is)
		for _, keyword := range keywords {
			if strings.Contains(keyword, " ") {
				// Multi-word keyword - check exact match in request portion (higher weight)
				if strings.Contains(requestPortion, keyword) {
					phraseMatches += 4 // Phrases in request portion count quadruple
					matches++
				} else if strings.Contains(messageLower, keyword) {
					// Also check full message but with lower weight
					phraseMatches += 2
					matches++
				} else {
					// Check if all words in the phrase appear in order (flexible matching)
					phraseWords := strings.Fields(keyword)
					if len(phraseWords) >= 2 {
						// Try flexible match in request portion first
						allWordsPresent := true
						lastIndex := -1
						for _, word := range phraseWords {
							idx := strings.Index(requestPortion[lastIndex+1:], word)
							if idx == -1 {
								allWordsPresent = false
								break
							}
							lastIndex = lastIndex + 1 + idx
						}
						if allWordsPresent {
							phraseMatches += 3 // Flexible phrase match in request portion
							matches++
						} else {
							// Try in full message
							allWordsPresent = true
							lastIndex = -1
							for _, word := range phraseWords {
								idx := strings.Index(messageLower[lastIndex+1:], word)
								if idx == -1 {
									allWordsPresent = false
									break
								}
								lastIndex = lastIndex + 1 + idx
							}
							if allWordsPresent {
								phraseMatches += 1 // Flexible phrase match in full message
								matches++
							}
						}
					}
				}
			}
		}

		// Second pass: check for single-word keywords (lower priority)
		// But give more weight to matches in request portion
		for _, keyword := range keywords {
			if !strings.Contains(keyword, " ") {
				// Single-word keyword
				matchedInRequest := strings.Contains(requestPortion, keyword)
				matchedInFull := strings.Contains(messageLower, keyword)

				if matchedInRequest || matchedInFull {
					// Check if this word is part of a phrase we already matched
					isPartOfPhrase := false
					for _, phraseKeyword := range keywords {
						if strings.Contains(phraseKeyword, " ") {
							phraseWords := strings.Fields(phraseKeyword)
							for _, word := range phraseWords {
								if word == keyword {
									// Check if the phrase matches in request portion
									if strings.Contains(requestPortion, phraseKeyword) {
										isPartOfPhrase = true
										break
									}
									// Check flexible match in request portion
									allWordsPresent := true
									lastIndex := -1
									for _, pw := range phraseWords {
										idx := strings.Index(requestPortion[lastIndex+1:], pw)
										if idx == -1 {
											allWordsPresent = false
											break
										}
										lastIndex = lastIndex + 1 + idx
									}
									if allWordsPresent {
										isPartOfPhrase = true
										break
									}
									// Check in full message
									if strings.Contains(messageLower, phraseKeyword) {
										isPartOfPhrase = true
										break
									}
								}
							}
							if isPartOfPhrase {
								break
							}
						}
					}
					if !isPartOfPhrase {
						if matchedInRequest {
							matches += 2 // Matches in request portion count double
						} else {
							matches++
						}
					}
				}
			}
		}

		if matches > 0 {
			// Calculate score: phrases weighted much more heavily
			baseScore := float64(matches) / float64(len(keywords))
			if phraseMatches > 0 {
				// Boost score significantly if we matched phrases (more specific)
				// Phrases are much more reliable indicators
				baseScore = baseScore * 2.0 // Double boost for phrases
				if baseScore > 1.0 {
					baseScore = 1.0
				}
			}
			scores[role] = baseScore
		}
	}

	// Check for code patterns (strong indicator for "code" role)
	hasCodePattern := false
	for _, pattern := range codePatterns {
		if pattern.MatchString(message) {
			hasCodePattern = true
			break
		}
	}
	if hasCodePattern {
		// Check if "code" role is available
		for _, availableRole := range availableRoles {
			if availableRole == "code" {
				scores["code"] = scores["code"] + 0.5
				if scores["code"] > 1.0 {
					scores["code"] = 1.0
				}
				break
			}
		}
	}

	// Check for shell command patterns (strong indicator for "shell" role)
	// But only if the message doesn't contain commit/branch keywords (to avoid false positives)
	hasCommitKeywords := strings.Contains(messageLower, "commit") || strings.Contains(messageLower, "changelog")
	hasBranchKeywords := strings.Contains(messageLower, "branch") && (strings.Contains(messageLower, "create") || strings.Contains(messageLower, "new") || strings.Contains(messageLower, "generate"))

	if !hasCommitKeywords && !hasBranchKeywords {
		if regexp.MustCompile(`^\s*\$?\s*[a-z]+(\s+[^\s]+)*\s*$`).MatchString(strings.TrimSpace(message)) &&
			len(messageWords) > 0 && len(messageWords) < 10 {
			for _, availableRole := range availableRoles {
				if availableRole == "shell" {
					scores["shell"] = scores["shell"] + 0.5
					if scores["shell"] > 1.0 {
						scores["shell"] = 1.0
					}
					break
				}
			}
		}
	}

	// Find the role with the highest score
	bestRole := ""
	bestScore := 0.0
	reason := ""
	for role, score := range scores {
		if score > bestScore {
			bestScore = score
			bestRole = role
		}
	}

	// Require a minimum score threshold to avoid false positives
	// Higher threshold for better precision
	minScoreThreshold := 0.4
	if bestRole == "describe" {
		// Describe role needs higher confidence since it can be confused with shell
		minScoreThreshold = 0.5
	}

	// If we have multiple roles with similar scores, prefer more specific ones
	// (commit/branch over shell/code when both match)
	// Commit and branch are more specific than shell/code
	if bestRole == "shell" || bestRole == "code" {
		// Check if commit or branch also scored well
		// Prefer commit/branch if they have a reasonable score (even if lower than shell/code)
		if commitScore, hasCommit := scores["commit"]; hasCommit && commitScore >= 0.2 {
			// If commit scored reasonably well, prefer it over shell/code
			// Even if shell/code scored higher, commit is more specific
			bestRole = "commit"
			bestScore = commitScore
		} else if branchScore, hasBranch := scores["branch"]; hasBranch && branchScore >= 0.2 {
			// If branch scored reasonably well, prefer it over shell/code
			bestRole = "branch"
			bestScore = branchScore
		}
	}

	if bestRole != "" && bestScore >= minScoreThreshold {
		keywords := getRoleKeywords(bestRole)
		reason = fmt.Sprintf("matched %d keywords with score %.2f", int(bestScore*float64(len(keywords))), bestScore)
		return bestRole, bestScore, reason
	}

	return "", 0.0, "no strong match found"
}

// detectRoleLLM uses an LLM to detect the most appropriate role
func detectRoleLLM(message string, availableRoles []string) (string, string, error) {
	// Build a prompt for role detection
	rolesList := strings.Join(availableRoles, ", ")
	prompt := fmt.Sprintf(`You are a role classifier for a CLI tool. Analyze the following user message and determine which role is most appropriate.

Available roles: %s

User message: %s

Respond with ONLY the role name (one word, lowercase) that best matches the user's intent. If none match well, respond with "default".

Role:`, rolesList, message)

	// Temporarily save current chat history
	oldHistory := GetChatHistory()
	ClearChatHistory()
	defer SetChatHistory(oldHistory)

	// Build request directly with default role to avoid recursion
	roleTemplate := viper.GetString("roles.default")
	systemContent := ""
	if roleTemplate != "" {
		systemContent = fmt.Sprintf(roleTemplate, os.Getenv("SHELL"), runtime.GOOS)
	}

	// Create a simple request for role detection (no history, just system + user)
	detectionRequest := APIRequest{
		Model: viper.GetString("model"),
		Messages: []Message{
			{Role: "system", Content: systemContent},
			{Role: "user", Content: prompt},
		},
		Stream: false, // Non-streaming for detection
	}

	// Get provider and send message directly
	provider, err := GetProvider()
	if err != nil {
		return "", "", fmt.Errorf("failed to get provider: %w", err)
	}

	// Check if model exists before sending
	if err := checkAndPullIfRequired(); err != nil {
		return "", "", fmt.Errorf("model check failed: %w", err)
	}

	// Send detection request (non-streaming, no printing)
	response, err := provider.SendMessage(detectionRequest, false)
	if err != nil {
		return "", "", fmt.Errorf("LLM detection failed: %w", err)
	}

	// Parse response - should be just the role name
	detectedRole := strings.ToLower(strings.TrimSpace(response))
	detectedRole = strings.Trim(detectedRole, "\"'`")
	// Remove any trailing punctuation or extra text
	fields := strings.Fields(detectedRole)
	if len(fields) > 0 {
		detectedRole = fields[0]
		if len(detectedRole) > 0 && (detectedRole[len(detectedRole)-1] == '.' || detectedRole[len(detectedRole)-1] == ',') {
			detectedRole = detectedRole[:len(detectedRole)-1]
		}
	}

	// Validate that the detected role is in available roles
	for _, role := range availableRoles {
		if role == detectedRole {
			return detectedRole, "LLM selected based on message analysis", nil
		}
	}

	// If not found, return default
	return "default", "LLM did not match any available role, using default", nil
}

// getAvailableRoles returns a list of available roles from configuration
func getAvailableRoles() []string {
	roles := []string{"default"} // default is always available
	allKeys := viper.AllKeys()
	for _, key := range allKeys {
		if strings.HasPrefix(key, "roles.") {
			roleName := strings.TrimPrefix(key, "roles.")
			if roleName != "" && roleName != "default" {
				// Check if role is not a nested key (e.g., "roles.git.commit" would be invalid)
				if !strings.Contains(roleName, ".") {
					roles = append(roles, roleName)
				}
			}
		}
	}
	return roles
}

// buildDetectionCacheKey creates a cache key for role detection results
func buildDetectionCacheKey(message string, availableRoles []string) (string, error) {
	payload := struct {
		Message        string   `json:"message"`
		AvailableRoles []string `json:"available_roles"`
	}{
		Message:        message,
		AvailableRoles: availableRoles,
	}

	var buf bytes.Buffer
	encoder := json.NewEncoder(&buf)
	if err := encoder.Encode(payload); err != nil {
		return "", fmt.Errorf("failed to encode detection cache key: %w", err)
	}
	sum := sha256.Sum256(buf.Bytes())
	return "detection_" + hex.EncodeToString(sum[:]), nil
}

// readDetectionCache reads a cached detection result
func readDetectionCache(key string) (*DetectionResult, bool, error) {
	cacheDir, err := getCacheDir()
	if err != nil {
		return nil, false, err
	}
	cachePath := filepath.Join(cacheDir, key+".json")
	data, err := os.ReadFile(cachePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, nil
		}
		return nil, false, err
	}

	var result DetectionResult
	if err := json.Unmarshal(data, &result); err != nil {
		return nil, false, err
	}
	return &result, true, nil
}

// writeDetectionCache writes a detection result to cache
func writeDetectionCache(key string, result *DetectionResult) error {
	cacheDir, err := getCacheDir()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(cacheDir, 0o755); err != nil {
		return fmt.Errorf("failed to create cache directory: %w", err)
	}

	data, err := json.Marshal(result)
	if err != nil {
		return fmt.Errorf("failed to encode detection result: %w", err)
	}
	cachePath := filepath.Join(cacheDir, key+".json")
	return os.WriteFile(cachePath, data, 0o600)
}

// DetectRole automatically detects the most appropriate role for a message
// Returns the detected role, method used, and any error
func DetectRole(message string, debug bool) (*DetectionResult, error) {
	// Check if auto-role is enabled
	if !viper.GetBool("auto_role.enabled") {
		return &DetectionResult{
			Role:   "default",
			Method: "default",
			Reason: "auto-role detection disabled",
		}, nil
	}

	// Check if explicit role is set (explicit role always wins)
	explicitRole := viper.GetString("systemrole")
	if explicitRole == "" {
		explicitRole = viper.GetString("role")
	}
	if explicitRole != "" {
		if debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] Using explicit role: %s\n", explicitRole)
		}
		return &DetectionResult{
			Role:   explicitRole,
			Method: "explicit",
			Reason: "role explicitly provided",
		}, nil
	}

	// Get available roles
	availableRoles := getAvailableRoles()

	// Check cache first
	if cacheEnabled() {
		cacheKey, err := buildDetectionCacheKey(message, availableRoles)
		if err == nil {
			if cached, ok, err := readDetectionCache(cacheKey); err == nil && ok {
				if debug {
					fmt.Fprintf(os.Stderr, "[DEBUG] Using cached role detection: %s (method: %s, reason: %s)\n",
						cached.Role, cached.Method, cached.Reason)
				}
				return cached, nil
			}
		}
	}

	// Get detection mode
	mode := viper.GetString("auto_role.mode")
	if mode == "" {
		mode = "hybrid"
	}

	var result *DetectionResult

	// Try heuristic first (if mode is heuristic or hybrid)
	if mode == "heuristic" || mode == "hybrid" {
		role, score, reason := detectRoleHeuristic(message, availableRoles)
		if role != "" && score > 0.3 {
			result = &DetectionResult{
				Role:   role,
				Method: "heuristic",
				Score:  score,
				Reason: reason,
			}
		}
	}

	// If heuristic didn't find a good match and mode is hybrid, try LLM
	if (result == nil || result.Role == "") && mode == "hybrid" {
		role, reason, err := detectRoleLLM(message, availableRoles)
		if err == nil {
			result = &DetectionResult{
				Role:   role,
				Method: "llm",
				Reason: reason,
			}
		} else if debug {
			fmt.Fprintf(os.Stderr, "[DEBUG] LLM detection failed: %v, falling back to default\n", err)
		}
	}

	// Fallback to default if nothing detected
	if result == nil || result.Role == "" {
		result = &DetectionResult{
			Role:   "default",
			Method: "default",
			Reason: "no role detected, using default",
		}
	}

	// Cache the result
	if cacheEnabled() {
		cacheKey, err := buildDetectionCacheKey(message, availableRoles)
		if err == nil {
			_ = writeDetectionCache(cacheKey, result)
		}
	}

	if debug {
		fmt.Fprintf(os.Stderr, "[DEBUG] Auto-detected role: %s (method: %s", result.Role, result.Method)
		if result.Score > 0 {
			fmt.Fprintf(os.Stderr, ", score: %.2f", result.Score)
		}
		if result.Reason != "" {
			fmt.Fprintf(os.Stderr, ", reason: %s", result.Reason)
		}
		fmt.Fprintf(os.Stderr, ")\n")
	}

	return result, nil
}
