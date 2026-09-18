package agent

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"gaia/plugins/ask"
	sanitizepkg "gaia/plugins/shared/sanitize"
)

// A run's window is fixed before it starts, and every file it reads stays in
// the conversation for the rest of it. Something has to go.

func aConversation(toolResults int) []ask.ChatMessage {
	msgs := []ask.ChatMessage{{Role: "user", Content: "fix the bug in inject.go"}}
	for i := 0; i < toolResults; i++ {
		msgs = append(msgs,
			ask.ChatMessage{Role: "assistant", Content: "reading it"},
			ask.ChatMessage{Role: "tool", ToolName: "read_file",
				Content: strings.Repeat("a line of a source file\n", 200)})
	}
	return msgs
}

func TestAConversationInsideItsBudgetIsUntouched(t *testing.T) {
	msgs := aConversation(1)

	require.Equal(t, msgs, trimConversation(msgs, 100_000))
}

func TestWithNoBudgetNothingIsTrimmed(t *testing.T) {
	msgs := aConversation(4)

	require.Equal(t, msgs, trimConversation(msgs, 0))
}

func TestAConversationOverItsBudgetComesBackUnderIt(t *testing.T) {
	msgs := aConversation(6)
	budget := 4000
	require.Greater(t, conversationTokens(msgs), budget)

	trimmed := trimConversation(msgs, budget)

	require.LessOrEqual(t, conversationTokens(trimmed), budget)
	require.Len(t, trimmed, len(msgs), "trimming replaces, it does not drop turns")
}

// The task is what the whole run is for; losing it is losing everything.
func TestTheTaskIsNeverTrimmed(t *testing.T) {
	msgs := aConversation(8)

	trimmed := trimConversation(msgs, 500)

	require.Equal(t, "fix the bug in inject.go", trimmed[0].Content)
}

// The recent turns are what the model is acting on right now.
func TestTheRecentTurnsAreKeptWhole(t *testing.T) {
	msgs := aConversation(8)

	trimmed := trimConversation(msgs, 500)

	for i := len(trimmed) - keepRecentTurns; i < len(trimmed); i++ {
		require.Equal(t, msgs[i].Content, trimmed[i].Content, "message %d", i)
	}
}

// What was dropped is named, so the model can decide to fetch it again rather
// than concluding the file was empty.
func TestWhatWasDroppedSaysWhatItWas(t *testing.T) {
	trimmed := trimConversation(aConversation(8), 500)

	var dropped string
	for _, msg := range trimmed {
		if strings.Contains(msg.Content, "dropped to make room") {
			dropped = msg.Content
			break
		}
	}
	require.NotEmpty(t, dropped)
	require.Contains(t, dropped, "read_file")
	require.Contains(t, dropped, "201 lines")
	require.Contains(t, dropped, "Call it again")
}

// Only tool output goes. The assistant's own turns carry the thread and cost
// almost nothing.
func TestTheModelsOwnTurnsAreLeftAlone(t *testing.T) {
	msgs := aConversation(8)

	trimmed := trimConversation(msgs, 500)

	for i, msg := range trimmed {
		if msgs[i].Role == "assistant" {
			require.Equal(t, msgs[i].Content, msg.Content)
		}
	}
}

// A short tool result is not worth replacing with a line about itself.
func TestASmallResultIsNotWorthDropping(t *testing.T) {
	msgs := []ask.ChatMessage{
		{Role: "user", Content: "a task"},
		{Role: "tool", ToolName: "run_command", Content: "exit code: 0"},
	}
	msgs = append(msgs, aConversation(4)[1:]...)

	trimmed := trimConversation(msgs, 500)

	require.Equal(t, "exit code: 0", trimmed[1].Content)
}

func TestTheBudgetIsWhatTheWindowHasLeftOver(t *testing.T) {
	rules := strings.Repeat("a rule of how to work. ", 100)
	tools := []ask.ToolSpec{{Name: "read_file", Description: strings.Repeat("x", 400)}}

	budget := ask.ConversationBudget(8192, rules, tools)

	require.Positive(t, budget)
	require.Less(t, budget, 8192)
	require.Greater(t, budget, 8192-2048-sanitizepkg.EstimateTokens(rules)-500)
}

func TestAWindowTooSmallToHoldTheRulesLeavesNoBudget(t *testing.T) {
	require.Zero(t, ask.ConversationBudget(1024, strings.Repeat("x", 40_000), nil))
	require.Zero(t, ask.ConversationBudget(0, "", nil))
}
