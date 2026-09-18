package ask

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gaia/plugins/mempalace"
	"gaia/plugins/shared"
	sanitizepkg "gaia/plugins/shared/sanitize"
)

// ApplySanitize rewrites the messages per sanitize config; errOut takes debug lines.
func ApplySanitize(errOut io.Writer, req AskRequest) AskRequest {
	if !viper.GetBool("sanitize.enabled") {
		return req
	}
	levelStr := strings.ToLower(strings.TrimSpace(viper.GetString("sanitize.level")))
	var level sanitizepkg.Level
	switch levelStr {
	case "none":
		level = sanitizepkg.LevelNone
	case "aggressive":
		level = sanitizepkg.LevelAggressive
	default:
		level = sanitizepkg.LevelLight
	}
	opts := sanitizepkg.Options{
		Level:             level,
		MaxTokensAfter:    viper.GetInt("sanitize.max_tokens_after"),
		LogStats:          viper.GetBool("sanitize.log_stats"),
		PreserveLastUser:  true,
		MaxDurationMillis: 100,
	}
	raw := buildMessagesForSanitize(req)
	out, stats, err := sanitizepkg.Sanitize(sanitizepkg.Request{Messages: raw}, opts)
	if err != nil {
		if viper.GetBool("debug") {
			_ = shared.PrintRaw(errOut, fmt.Sprintf("[DEBUG] sanitize: %v\n", err))
		}
		return req
	}
	if opts.LogStats && (stats.TokensBefore > 0 || stats.TokensAfter > 0) {
		_ = shared.PrintRaw(errOut,
			fmt.Sprintf("[sanitize] tokens before=%d after=%d removed≈%d ms=%d\n",
				stats.TokensBefore, stats.TokensAfter, stats.RemovedCount, stats.DurationMillis))
	}
	req.SystemPrompt = ""
	req.Message = ""
	req.Messages = make([]ChatMessage, 0, len(out.Messages))
	for _, m := range out.Messages {
		req.Messages = append(req.Messages, ChatMessage{Role: m.Role, Content: m.Content})
	}
	return req
}

func buildMessagesForSanitize(req AskRequest) []sanitizepkg.Message {
	out := []sanitizepkg.Message{}
	if strings.TrimSpace(req.SystemPrompt) != "" {
		out = append(out, sanitizepkg.Message{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		if strings.TrimSpace(m.Role) == "" || strings.TrimSpace(m.Content) == "" {
			continue
		}
		out = append(out, sanitizepkg.Message{Role: m.Role, Content: m.Content})
	}
	if strings.TrimSpace(req.Message) != "" {
		out = append(out, sanitizepkg.Message{Role: "user", Content: req.Message})
	}
	return out
}

// resolveSystemPrompt prefers MemPalace context, falling back to a role.
func resolveSystemPrompt(cmd *cobra.Command, req *AskRequest, msg string) error {
	ctxPrompt, err := mempalace.SearchContextIfEnabled(cmd.Context(), msg)
	if err != nil {
		return err
	}
	if ctxPrompt != "" {
		req.SystemPrompt = ctxPrompt
		return nil
	}
	return applyRolePrompt(cmd, req, msg)
}

func applyRolePrompt(cmd *cobra.Command, req *AskRequest, msg string) error {
	prompt, err := RolePromptFor(cmd, viper.GetString("ask.role"), msg, *req)
	if err != nil {
		return err
	}
	if prompt != "" {
		req.SystemPrompt = prompt
	}
	return nil
}
