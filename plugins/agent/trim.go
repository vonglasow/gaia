package agent

import (
	"fmt"

	"gaia/plugins/ask"
	sanitizepkg "gaia/plugins/shared/sanitize"
)

// A run's window is fixed before it starts, and every file it reads stays in
// the conversation for the rest of it. Something has to go, and what goes is
// old tool output: the model has already acted on it, and can read it again.

// keepRecentTurns is how much of the conversation stays verbatim. Two turns is
// the tool result just acted on, and the one before it.
const keepRecentTurns = 6

// trimConversation brings the conversation under budget by replacing old tool
// output with a line saying what it was. The task and the recent turns are
// never touched: a model that loses either has nothing left to work from.
func trimConversation(messages []ask.ChatMessage, budget int) []ask.ChatMessage {
	if budget <= 0 || conversationTokens(messages) <= budget {
		return messages
	}

	out := make([]ask.ChatMessage, len(messages))
	copy(out, messages)

	// Oldest first, and never the task at index 0 or the recent tail.
	last := len(out) - keepRecentTurns
	for i := 1; i < last; i++ {
		if out[i].Role != "tool" || out[i].Content == "" {
			continue
		}
		if sanitizepkg.EstimateTokens(out[i].Content) <= 40 {
			continue
		}
		out[i].Content = dropped(out[i])
		if conversationTokens(out) <= budget {
			break
		}
	}
	return out
}

// dropped says what was there, so the model can decide to fetch it again.
func dropped(msg ask.ChatMessage) string {
	name := msg.ToolName
	if name == "" {
		name = "a tool"
	}
	return fmt.Sprintf("[%s output dropped to make room — %d lines. Call it again if you need it.]",
		name, countLines(msg.Content))
}

func countLines(s string) int {
	n := 1
	for _, r := range s {
		if r == '\n' {
			n++
		}
	}
	return n
}

func conversationTokens(messages []ask.ChatMessage) int {
	total := 0
	for _, msg := range messages {
		total += sanitizepkg.EstimateTokens(msg.Content)
	}
	return total
}
