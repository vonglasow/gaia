package api

import (
	"fmt"
	"os"
	"strings"

	"gaia/api/sanitize"

	"github.com/spf13/viper"
)

// applySanitizeIfEnabled runs sanitization on request messages when sanitize_before_llm is true.
// Returns the request (possibly with sanitized messages) and whether sanitization was applied.
func applySanitizeIfEnabled(request APIRequest) (APIRequest, bool) {
	if !viper.GetBool("sanitize_before_llm") {
		return request, false
	}
	levelStr := strings.ToLower(strings.TrimSpace(viper.GetString("sanitize.level")))
	var level sanitize.Level
	switch levelStr {
	case "", "none":
		level = sanitize.LevelNone
	case "light":
		level = sanitize.LevelLight
	case "aggressive":
		level = sanitize.LevelAggressive
	default:
		level = sanitize.LevelLight
	}
	opts := sanitize.Options{
		Level:             level,
		MaxTokensAfter:    viper.GetInt("sanitize.max_tokens_after"),
		LogStats:          viper.GetBool("sanitize.log_stats"),
		PreserveLastUser:  true,
		MaxDurationMillis: 100,
	}
	sreq := sanitize.Request{Messages: make([]sanitize.Message, len(request.Messages))}
	for i, m := range request.Messages {
		sreq.Messages[i] = sanitize.Message{Role: m.Role, Content: m.Content}
	}
	out, stats, err := sanitize.Sanitize(sreq, opts)
	if err != nil {
		if viper.GetBool("debug") {
			fmt.Fprintf(os.Stderr, "[DEBUG] sanitize: %v\n", err)
		}
		return request, false
	}
	request.Messages = make([]Message, len(out.Messages))
	for i, m := range out.Messages {
		request.Messages[i] = Message{Role: m.Role, Content: m.Content}
	}
	if opts.LogStats && (stats.TokensBefore > 0 || stats.TokensAfter > 0) {
		fmt.Fprintf(os.Stderr, "[sanitize] tokens before=%d after=%d removed≈%d ms=%d\n",
			stats.TokensBefore, stats.TokensAfter, stats.RemovedCount, stats.DurationMillis)
	}
	return request, true
}
