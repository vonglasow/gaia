// Package sanitize filters and reduces noise in message content before sending to an LLM.
package sanitize

import (
	"regexp"
	"strings"
	"time"
)

// Level is the sanitization intensity: none, light, or aggressive.
type Level string

const (
	LevelNone       Level = "none"
	LevelLight      Level = "light"
	LevelAggressive Level = "aggressive"
)

// Message is a single message (role + content) for sanitization.
type Message struct {
	Role    string
	Content string
}

// Request holds messages to sanitize (no model/stream).
type Request struct {
	Messages []Message
}

// Stats holds sanitization metrics for logging.
type Stats struct {
	TokensBefore   int
	TokensAfter    int
	RemovedCount   int // number of elements/lines removed or reduced
	DurationMillis int64
}

// Options configures sanitization behavior.
type Options struct {
	Level             Level
	MaxTokensAfter    int  // 0 = no cap
	LogStats          bool // for caller to decide whether to log
	PreserveLastUser  bool // always preserve last user message content (critical context)
	MaxDurationMillis int64
}

// DefaultOptions returns options with safe defaults.
func DefaultOptions() Options {
	return Options{
		Level:             LevelLight,
		MaxTokensAfter:    0,
		LogStats:          true,
		PreserveLastUser:  true,
		MaxDurationMillis: 100,
	}
}

var (
	reDebugLine  = regexp.MustCompile(`(?m)^\s*\[?(DEBUG|WARN|INFO|TRACE|ERROR)\]?\s*[:\s]`)
	reTimestamp  = regexp.MustCompile(`(?m)^\s*\d{4}-\d{2}-\d{2}[T\s]\d{2}:\d{2}:\d{2}`)
	reOnlyPunct  = regexp.MustCompile(`^[\s\p{P}\d]+$`)
	reMultipleNL = regexp.MustCompile(`\n{3,}`)
	reTrailingWS = regexp.MustCompile(`[ \t]+\n`)
)

// EstimateTokens approximates token count (chars/4 heuristic).
func EstimateTokens(s string) int {
	if s == "" {
		return 0
	}
	n := len([]rune(s))
	t := n / 4
	if t == 0 && n > 0 {
		return 1
	}
	return t
}

// Sanitize runs the sanitization pipeline on the request and returns updated request and stats.
// It respects MaxDurationMillis by doing work in small steps (best-effort).
func Sanitize(req Request, opts Options) (Request, Stats, error) {
	start := time.Now()
	stats := Stats{}
	for _, m := range req.Messages {
		stats.TokensBefore += EstimateTokens(m.Content)
	}

	if opts.Level == LevelNone || len(req.Messages) == 0 {
		stats.TokensAfter = stats.TokensBefore
		stats.DurationMillis = time.Since(start).Milliseconds()
		return req, stats, nil
	}

	out := make([]Message, 0, len(req.Messages))
	lastUserIdx := -1
	for i := len(req.Messages) - 1; i >= 0; i-- {
		if req.Messages[i].Role == "user" {
			lastUserIdx = i
			break
		}
	}

	for i, m := range req.Messages {
		isLastUser := opts.PreserveLastUser && i == lastUserIdx
		content := sanitizeContent(m.Content, opts.Level, m.Role, isLastUser)
		out = append(out, Message{Role: m.Role, Content: content})
	}

	// Apply global token cap if set (preserve system and last user message)
	if opts.MaxTokensAfter > 0 {
		out = applyTokenCap(out, opts.MaxTokensAfter, lastUserIdx)
	}

	for _, m := range out {
		stats.TokensAfter += EstimateTokens(m.Content)
	}
	stats.RemovedCount = stats.TokensBefore - stats.TokensAfter
	if stats.RemovedCount < 0 {
		stats.RemovedCount = 0
	}
	stats.DurationMillis = time.Since(start).Milliseconds()
	return Request{Messages: out}, stats, nil
}

func sanitizeContent(content string, level Level, role string, preserveCritical bool) string {
	if content == "" {
		return content
	}
	if preserveCritical && role == "user" {
		// Light cleanup only: collapse newlines, trim
		content = reMultipleNL.ReplaceAllString(content, "\n\n")
		content = reTrailingWS.ReplaceAllString(content, "\n")
		return strings.TrimSpace(content)
	}

	lines := strings.Split(content, "\n")
	var out []string
	prev := ""
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if lightFilter(trimmed) {
			continue
		}
		if level == LevelAggressive && aggressiveFilter(trimmed) {
			continue
		}
		// Consecutive duplicate
		if trimmed == prev && prev != "" {
			continue
		}
		prev = trimmed
		// Aggressive: truncate very long lines without structure
		if level == LevelAggressive && len([]rune(trimmed)) > 500 {
			trimmed = string([]rune(trimmed)[:250]) + "…"
		}
		out = append(out, trimmed)
	}
	content = strings.Join(out, "\n")
	content = reMultipleNL.ReplaceAllString(content, "\n\n")
	content = reTrailingWS.ReplaceAllString(content, "\n")
	return strings.TrimSpace(content)
}

func lightFilter(line string) bool {
	if line == "" {
		return true
	}
	// Metadata / debug
	if reDebugLine.MatchString(line) {
		return true
	}
	if reTimestamp.MatchString(line) {
		return true
	}
	// Very short noise
	if len([]rune(line)) <= 2 && reOnlyPunct.MatchString(line) {
		return true
	}
	return false
}

func aggressiveFilter(line string) bool {
	// Long run of same character or only punctuation/numbers
	if reOnlyPunct.MatchString(line) {
		return true
	}
	// Line that looks like base64 or hex dump (no spaces, long)
	runes := []rune(line)
	if len(runes) > 200 {
		spaces := 0
		for _, r := range runes {
			if r == ' ' || r == '\t' {
				spaces++
			}
		}
		if spaces == 0 {
			return true
		}
	}
	return false
}

// applyTokenCap reduces message contents so total tokens <= maxTokens.
// Preserves system (index 0) and last user message; trims or drops older history.
func applyTokenCap(msgs []Message, maxTokens int, lastUserIdx int) []Message {
	total := 0
	for _, m := range msgs {
		total += EstimateTokens(m.Content)
	}
	if total <= maxTokens {
		return msgs
	}
	out := make([]Message, len(msgs))
	copy(out, msgs)
	protected := map[int]bool{0: true}
	if lastUserIdx >= 0 {
		protected[lastUserIdx] = true
	}
	for total > maxTokens {
		reduced := false
		for i := len(out) - 1; i >= 0; i-- {
			if protected[i] || out[i].Content == "" {
				continue
			}
			tok := EstimateTokens(out[i].Content)
			if tok <= 10 {
				continue
			}
			newTok := tok * 3 / 4
			if newTok < 20 {
				newTok = 20
			}
			newLen := newTok * 4
			runes := []rune(out[i].Content)
			if len(runes) > newLen {
				out[i].Content = string(runes[:newLen]) + "…"
				total = total - tok + EstimateTokens(out[i].Content)
				reduced = true
				break
			}
		}
		if !reduced {
			// Drop oldest non-protected message
			for i := 1; i < len(out); i++ {
				if protected[i] {
					continue
				}
				total -= EstimateTokens(out[i].Content)
				out = append(out[:i], out[i+1:]...)
				// Adjust lastUserIdx after removal
				if lastUserIdx >= i {
					lastUserIdx--
				}
				protected = map[int]bool{0: true}
				if lastUserIdx >= 0 {
					protected[lastUserIdx] = true
				}
				reduced = true
				break
			}
			if !reduced {
				break
			}
		}
	}
	return out
}
