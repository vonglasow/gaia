package sanitize

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Written against mutation testing, which reported 60% efficacy on a package
// covered at 97%: the lines below all ran, and nothing asserted on what they
// produced. Each test here killed a mutant that had survived.

// A short string is one token, not none: the caller divides a budget by this,
// and zero for a non-empty message means an unbounded one.
func TestATinyStringCostsOneTokenRatherThanNone(t *testing.T) {
	require.Equal(t, 1, EstimateTokens("a"))
	require.Equal(t, 1, EstimateTokens("abc"))
	require.Equal(t, 1, EstimateTokens("abcd"))
	require.Equal(t, 2, EstimateTokens("abcdefgh"))
	require.Zero(t, EstimateTokens(""))
}

// The last user turn is found by walking backwards, so a later one wins.
func TestTheLastUserTurnIsTheLastOneAndNotTheFirst(t *testing.T) {
	out, _, err := Sanitize(Request{Messages: []Message{
		{Role: "user", Content: "[DEBUG] first"},
		{Role: "assistant", Content: "[DEBUG] middle"},
		{Role: "user", Content: "[DEBUG] last"},
	}}, Options{Level: LevelLight, PreserveLastUser: true})

	require.NoError(t, err)
	require.Empty(t, out.Messages[0].Content, "an earlier user turn is filtered")
	require.Contains(t, out.Messages[2].Content, "[DEBUG] last", "the last one is preserved")
}

// With nobody to preserve, every turn is filtered — including user turns.
func TestWithPreserveOffEveryTurnIsFiltered(t *testing.T) {
	out, _, err := Sanitize(Request{Messages: []Message{
		{Role: "user", Content: "[DEBUG] noise"},
	}}, Options{Level: LevelLight, PreserveLastUser: false})

	require.NoError(t, err)
	require.Empty(t, out.Messages[0].Content)
}

// Only a user turn is preserved. An assistant turn at the same index is not.
func TestOnlyAUserTurnIsPreserved(t *testing.T) {
	out, _, err := Sanitize(Request{Messages: []Message{
		{Role: "assistant", Content: "[DEBUG] noise"},
	}}, Options{Level: LevelLight, PreserveLastUser: true})

	require.NoError(t, err)
	require.Empty(t, out.Messages[0].Content, "there is no user turn to preserve")
}

// The counts are what a caller logs, so they have to be the real ones.
func TestTheStatsCountWhatWasRemoved(t *testing.T) {
	noise := strings.Repeat("[DEBUG] a line of noise\n", 20)
	_, stats, err := Sanitize(Request{Messages: []Message{
		{Role: "assistant", Content: noise},
	}}, Options{Level: LevelLight})

	require.NoError(t, err)
	require.Positive(t, stats.TokensBefore)
	require.Less(t, stats.TokensAfter, stats.TokensBefore)
	require.Equal(t, stats.TokensBefore-stats.TokensAfter, stats.RemovedCount)
}

// Sanitising can make a message longer than it was — a short one gains nothing
// and loses nothing — and a negative "removed" would read as text invented.
func TestRemovedCountIsNeverNegative(t *testing.T) {
	_, stats, err := Sanitize(Request{Messages: []Message{
		{Role: "user", Content: "a short question"},
	}}, Options{Level: LevelLight, PreserveLastUser: true})

	require.NoError(t, err)
	require.GreaterOrEqual(t, stats.RemovedCount, 0)
}

// A cap of zero is no cap, which is the default: turning it on is a decision.
func TestACapOfZeroLeavesTheConversationAlone(t *testing.T) {
	long := strings.TrimSpace(strings.Repeat("word ", 400))
	out, _, err := Sanitize(Request{Messages: []Message{
		{Role: "system", Content: "be brief"},
		{Role: "assistant", Content: long},
		{Role: "user", Content: "and now"},
	}}, Options{Level: LevelLight, MaxTokensAfter: 0, PreserveLastUser: true})

	require.NoError(t, err)
	require.Equal(t, long, out.Messages[1].Content)
}

// LevelNone returns the request untouched, which is what "off" has to mean.
func TestTheNoneLevelChangesNothing(t *testing.T) {
	req := Request{Messages: []Message{{Role: "assistant", Content: "[DEBUG] noise\nreal"}}}

	out, stats, err := Sanitize(req, Options{Level: LevelNone})

	require.NoError(t, err)
	require.Equal(t, req.Messages, out.Messages)
	require.Zero(t, stats.RemovedCount)
}

func TestAnEmptyConversationIsNotAFailure(t *testing.T) {
	out, stats, err := Sanitize(Request{}, Options{Level: LevelLight})

	require.NoError(t, err)
	require.Empty(t, out.Messages)
	require.Zero(t, stats.TokensBefore)
}

// --- what the filters cut, exactly ---------------------------------------

// Two characters of punctuation is noise; three is a line somebody wrote.
func TestTheShortNoiseRuleCutsAtTwoCharacters(t *testing.T) {
	require.True(t, lightFilter("--"))
	require.True(t, lightFilter("."))
	require.False(t, lightFilter("---"), "three is a separator somebody typed")
	require.False(t, lightFilter("ab"), "letters are not punctuation")
}

// Aggressive drops an unbroken run over 200 characters and keeps 200 exactly.
func TestTheBlobRuleCutsAboveTwoHundred(t *testing.T) {
	require.False(t, aggressiveFilter(strings.Repeat("a", 200)))
	require.True(t, aggressiveFilter(strings.Repeat("a", 201)))
	require.False(t, aggressiveFilter(strings.Repeat("a ", 200)), "spaces make it prose")
}

// A line over 500 characters is truncated to 250, and one at 500 is not.
func TestAggressiveTruncatesTheVeryLongLinesOnly(t *testing.T) {
	long := strings.Repeat("a b ", 200) // 800 chars, with spaces
	out := sanitizeContent(long, LevelAggressive, "assistant", false)
	require.Contains(t, out, "…")
	require.Less(t, len([]rune(out)), 300)

	shorter := strings.Repeat("a b ", 100) // 400 chars
	require.NotContains(t, sanitizeContent(shorter, LevelAggressive, "assistant", false), "…")
}

// A line repeated twice running is one line; the same line later is not.
func TestOnlyConsecutiveDuplicatesAreCollapsed(t *testing.T) {
	out := sanitizeContent("same\nsame\nother\nsame", LevelLight, "assistant", false)

	require.Equal(t, 3, len(strings.Split(out, "\n")), "the repeat next to itself goes")
}

func TestAnEmptyContentIsReturnedAsItIs(t *testing.T) {
	require.Empty(t, sanitizeContent("", LevelLight, "user", true))
}
