package sanitize

import (
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// The token cap is what keeps a long conversation inside the model's context window.

func longMessage(role string, words int) Message {
	return Message{Role: role, Content: strings.TrimSpace(strings.Repeat("word ", words))}
}

func totalTokens(msgs []Message) int {
	total := 0
	for _, m := range msgs {
		total += EstimateTokens(m.Content)
	}
	return total
}

func TestAConversationUnderTheCapIsLeftAlone(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "be brief"},
		{Role: "user", Content: "a question"},
	}

	out := applyTokenCap(msgs, 10_000, 1)

	require.Equal(t, msgs, out, "a cap nothing exceeds must change nothing")
}

func TestAConversationOverTheCapIsBroughtUnderIt(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "be brief"},
		longMessage("user", 400),
		longMessage("assistant", 400),
		{Role: "user", Content: "and now summarise"},
	}
	require.Greater(t, totalTokens(msgs), 200)

	out := applyTokenCap(msgs, 200, 3)

	require.LessOrEqual(t, totalTokens(out), 200)
}

func TestTheSystemPromptSurvivesTheCap(t *testing.T) {
	system := "you are an operator and these are your rules"
	msgs := []Message{
		{Role: "system", Content: system},
		longMessage("assistant", 500),
		longMessage("assistant", 500),
		{Role: "user", Content: "and now summarise"},
	}

	out := applyTokenCap(msgs, 100, 3)

	require.Equal(t, system, out[0].Content,
		"trimming the rules the model runs under would change what it is allowed to do")
}

func TestTheLastUserTurnSurvivesTheCap(t *testing.T) {
	question := "and now summarise everything you were told above"
	msgs := []Message{
		{Role: "system", Content: "be brief"},
		longMessage("assistant", 500),
		longMessage("assistant", 500),
		{Role: "user", Content: question},
	}

	out := applyTokenCap(msgs, 100, 3)

	require.Equal(t, question, out[3].Content,
		"the question being answered is not history")
}

// What gets cut is the middle, and it gets cut rather than dropped.
func TestHistoryIsShortenedAndNotRemoved(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "be brief"},
		longMessage("user", 400),
		longMessage("assistant", 400),
		{Role: "user", Content: "and now summarise"},
	}

	out := applyTokenCap(msgs, 150, 3)

	require.Len(t, out, 4, "every turn is still there")
	require.Less(t, EstimateTokens(out[2].Content), EstimateTokens(msgs[2].Content))
}

// A conversation with no user turn at all.
func TestACapWithNoUserTurnStillTerminates(t *testing.T) {
	msgs := []Message{
		{Role: "system", Content: "be brief"},
		longMessage("assistant", 500),
	}

	out := applyTokenCap(msgs, 50, -1)

	require.Len(t, out, 2)
}

// A cap smaller than what the protected messages already cost cannot be met.
func TestACapSmallerThanWhatIsProtectedStillReturns(t *testing.T) {
	msgs := []Message{
		longMessage("system", 300),
		longMessage("assistant", 300),
		longMessage("user", 300),
	}

	done := make(chan []Message, 1)
	go func() { done <- applyTokenCap(msgs, 1, 2) }()

	select {
	case out := <-done:
		require.Len(t, out, 2,
			"unable to meet the cap, it drops the history and keeps the two it may not touch")
		require.Equal(t, "system", out[0].Role, "the system prompt is never dropped")
		require.Equal(t, "user", out[1].Role, "nor is the question being answered")
		require.NotEmpty(t, out[0].Content)
		require.NotEmpty(t, out[1].Content)
	case <-time.After(5 * time.Second):
		t.Fatal("applyTokenCap did not return: the cap loop is not making progress")
	}
}

func TestTheCapIsAppliedThroughSanitize(t *testing.T) {
	req := Request{Messages: []Message{
		{Role: "system", Content: "be brief"},
		longMessage("assistant", 500),
		{Role: "user", Content: "and now summarise"},
	}}

	out, stats, err := Sanitize(req, Options{
		Level:             LevelLight,
		MaxTokensAfter:    100,
		PreserveLastUser:  true,
		MaxDurationMillis: 100,
	})

	require.NoError(t, err)
	require.LessOrEqual(t, totalTokens(out.Messages), 100)
	require.Greater(t, stats.TokensBefore, stats.TokensAfter)
}

// --- what the filters actually drop ---------------------------------------

// Stated as a table because the name "sanitize" invites the wrong reading.
func TestTheLightFilterDropsNoiseAndKeepsContent(t *testing.T) {
	for line, dropped := range map[string]bool{
		"":                                      true,
		"[DEBUG] connecting":                    true,
		"WARN: retrying":                        true,
		"ERROR : something":                     true,
		"2026-09-16T10:00:00 started":           true,
		"2026-09-16 10:00:00 started":           true,
		"--":                                    true,
		"42":                                    true,
		"func main() {":                         false,
		"my api key is sk-abc123":               false,
		"write to someone@example.com about it": false,
	} {
		require.Equalf(t, dropped, lightFilter(line), "line %q", line)
	}
}

func TestTheAggressiveFilterAlsoDropsUnbrokenBlobs(t *testing.T) {
	blob := strings.Repeat("a1b2c3d4", 30)
	require.Greater(t, len(blob), 200)

	require.True(t, aggressiveFilter(blob), "a 240-character run with no space is a dump, not prose")
	require.False(t, aggressiveFilter("a sentence that happens to be quite long "+strings.Repeat("and longer ", 20)),
		"length alone is not the test; the absence of spaces is")
	require.True(t, aggressiveFilter("--------"))
	require.False(t, aggressiveFilter("func main() {"))
}

func TestEstimateTokensGrowsWithTheText(t *testing.T) {
	require.Zero(t, EstimateTokens(""))
	require.Positive(t, EstimateTokens("a short sentence"))
	require.Greater(t,
		EstimateTokens(strings.Repeat("word ", 100)),
		EstimateTokens(strings.Repeat("word ", 10)))
}

func TestDefaultOptionsProtectTheLastUserTurn(t *testing.T) {
	opts := DefaultOptions()

	require.True(t, opts.PreserveLastUser)
	require.Equal(t, LevelLight, opts.Level)
	require.Zero(t, opts.MaxTokensAfter, "no cap unless one is asked for")
}
