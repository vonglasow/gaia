package ask

import (
	"encoding/json"

	"github.com/spf13/viper"

	sanitizepkg "gaia/plugins/shared/sanitize"
)

// Ollama's own default window is 4096 tokens, and it does not complain when a
// request exceeds it: it drops the oldest messages, which are the system prompt
// and the task. A single source file is enough to do that, and what comes back
// is a model answering about the last thing it can still see.

// ollamaDefaultWindow is what Ollama uses when nobody says otherwise.
const ollamaDefaultWindow = 4096

// maxContextWindow caps what gaia will ask for on its own. A window is memory:
// too large a one pushes a model off the GPU, which is slower than any saving.
const maxContextWindow = 32768

// answerHeadroom is room for the reply, on top of what is being sent.
const answerHeadroom = 2048

// contextWindow sizes the window to what is actually being sent, in the buckets
// Ollama allocates in.
func contextWindow(req AskRequest) int {
	if configured := viper.GetInt("ollama.num_ctx"); configured > 0 {
		return configured
	}
	if req.ContextWindow > 0 {
		return req.ContextWindow
	}
	needed := promptTokens(req) + answerHeadroom
	limit := viper.GetInt("ollama.max_num_ctx")
	if limit <= 0 {
		limit = maxContextWindow
	}
	window := ollamaDefaultWindow
	for window < needed {
		window *= 2
		if window >= limit {
			return limit
		}
	}
	return window
}

// promptTokens estimates everything that goes out: the rules, the conversation,
// and the tool schemas, which are not free either.
func promptTokens(req AskRequest) int {
	total := sanitizepkg.EstimateTokens(req.SystemPrompt) + sanitizepkg.EstimateTokens(req.Message)
	for _, msg := range req.Messages {
		total += sanitizepkg.EstimateTokens(msg.Content)
	}
	if len(req.Tools) > 0 {
		if encoded, err := json.Marshal(toOllamaTools(req.Tools)); err == nil {
			total += sanitizepkg.EstimateTokens(string(encoded))
		}
	}
	return total
}

// WindowForRun sizes one window for a whole run, before the reads that will
// fill it. Ollama reloads the model whenever num_ctx changes, which costs
// seconds a turn, and a window wide enough to matter can push the model off
// the GPU — so it is decided once, here, rather than per turn.
func WindowForRun(req AskRequest) int {
	if configured := viper.GetInt("ollama.num_ctx"); configured > 0 {
		return configured
	}
	// One bucket above the opening turn: the rules and the tool schemas are
	// what every turn carries, and the reads land on top of them.
	return contextWindow(AskRequest{
		SystemPrompt: req.SystemPrompt,
		Message:      req.Message,
		Messages:     req.Messages,
		Tools:        req.Tools,
	}) * 2
}

// ConversationBudget is what is left of a window once the rules, the tool
// schemas and room for an answer are taken out. It is what the turns so far
// may occupy between them.
func ConversationBudget(window int, systemPrompt string, tools []ToolSpec) int {
	if window <= 0 {
		return 0
	}
	fixed := promptTokens(AskRequest{SystemPrompt: systemPrompt, Tools: tools})
	budget := window - fixed - answerHeadroom
	if budget < 0 {
		return 0
	}
	return budget
}
