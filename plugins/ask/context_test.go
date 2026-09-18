package ask

import (
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// Ollama's window is 4096 and it does not complain when a request exceeds it:
// it drops the oldest messages, which are the system prompt and the task. A
// model then answers about the last thing it can still see, which is why an
// agent asked to change a file came back describing it.

func TestAShortExchangeGetsTheOrdinaryWindow(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.Equal(t, 4096, contextWindow(AskRequest{Message: "what is 2 + 2"}))
}

// A source file is enough on its own to need more than the default.
func TestReadingAFileWidensTheWindow(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	aFile := strings.Repeat("\tfmt.Println(\"a line of a real file\")\n", 400)

	got := contextWindow(AskRequest{Messages: []ChatMessage{{Role: "tool", Content: aFile}}})

	require.Greater(t, got, 4096)
}

func TestTheWindowGrowsInTheBucketsOllamaAllocatesIn(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	for _, size := range []int{1, 20000, 50000, 100000} {
		got := contextWindow(AskRequest{Message: strings.Repeat("x", size)})
		require.Contains(t, []int{4096, 8192, 16384, 32768}, got, "for %d chars", size)
	}
}

// A window is memory. Too large a one pushes a model off the GPU, which costs
// more than it saves.
func TestTheWindowIsCapped(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.Equal(t, 32768, contextWindow(AskRequest{Message: strings.Repeat("x", 5_000_000)}))
}

func TestTheCapCanBeLowered(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ollama.max_num_ctx", 8192)

	require.Equal(t, 8192, contextWindow(AskRequest{Message: strings.Repeat("x", 5_000_000)}))
}

// A machine with little memory, or one with a lot, is told outright.
func TestAConfiguredWindowIsUsedAsItIs(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ollama.num_ctx", 65536)

	require.Equal(t, 65536, contextWindow(AskRequest{Message: "short"}))
}

// The tool schemas are sent on every turn and are not free.
func TestTheToolSchemasCountTowardsTheWindow(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	req := AskRequest{Message: strings.Repeat("x", 7000)}
	withTools := req
	withTools.Tools = []ToolSpec{{
		Name:        "read_file",
		Description: strings.Repeat("a description of what this tool does. ", 200),
		Parameters:  map[string]any{"type": "object"},
	}}

	require.Greater(t, promptTokens(withTools), promptTokens(req))
}

// Ollama reloads the model whenever num_ctx changes — measured at 6.8 seconds
// against 0.3 for the same request at the same size. A window that grows turn
// by turn pays that every time it grows.

func TestAFixedWindowIsUsedAsGiven(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	got := contextWindow(AskRequest{ContextWindow: 16384, Message: "short"})

	require.Equal(t, 16384, got)
}

// A run is sized before the reads that will fill it, not after each one.
func TestARunIsSizedAboveItsOpeningTurn(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	opening := AskRequest{SystemPrompt: strings.Repeat("a rule. ", 200), Message: "a task"}

	require.Equal(t, contextWindow(opening)*2, WindowForRun(opening))
}

func TestAConfiguredWindowOverridesTheRunSizing(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ollama.num_ctx", 12288)

	require.Equal(t, 12288, WindowForRun(AskRequest{Message: "a task"}))
	require.Equal(t, 12288, contextWindow(AskRequest{ContextWindow: 16384}))
}
