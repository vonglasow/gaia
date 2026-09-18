package mempalace

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// MemPalace is a separate process gaia talks to over MCP.

type recordedCall struct {
	name string
	args map[string]interface{}
}

// stubMCP replaces the transport for one test and records what gaia asked for.
func stubMCP(t *testing.T, answer string, failWith error) *[]recordedCall {
	t.Helper()
	calls := &[]recordedCall{}
	previous := callToolFn
	callToolFn = func(_ context.Context, name string, args map[string]interface{}) (json.RawMessage, error) {
		*calls = append(*calls, recordedCall{name: name, args: args})
		if failWith != nil {
			return nil, failWith
		}
		return json.RawMessage(answer), nil
	}
	t.Cleanup(func() { callToolFn = previous })
	return calls
}

func contextEnabled(t *testing.T) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("mempalace.context.enabled", true)
}

// The feature is off by default, and off means silent.
func TestNoSearchHappensWhileTheContextFeatureIsOff(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	calls := stubMCP(t, `{}`, nil)

	out, err := SearchContextIfEnabled(context.Background(), "a question")

	require.NoError(t, err)
	require.Empty(t, out)
	require.Empty(t, *calls, "nothing was asked of a tool nobody enabled")
}

func TestAnEmptyQueryIsNotWorthASearch(t *testing.T) {
	contextEnabled(t)
	calls := stubMCP(t, `{}`, nil)

	out, err := SearchContextIfEnabled(context.Background(), "   ")

	require.NoError(t, err)
	require.Empty(t, out)
	require.Empty(t, *calls)
}

func TestTheSearchCarriesTheConfiguredWingAndRoom(t *testing.T) {
	contextEnabled(t)
	viper.Set("mempalace.context.wing", "gaia")
	viper.Set("mempalace.context.room", "feedback")
	viper.Set("mempalace.context.max_results", 3)
	viper.Set("mempalace.context.min_score", 0.5)
	calls := stubMCP(t, `{"results":[]}`, nil)

	_, err := SearchContextIfEnabled(context.Background(), "a question")

	require.NoError(t, err)
	require.Len(t, *calls, 1)
	call := (*calls)[0]
	require.Equal(t, "mempalace_search", call.name)
	require.Equal(t, "a question", call.args["query"])
	require.Equal(t, "gaia", call.args["wing"])
	require.Equal(t, "feedback", call.args["room"])
	require.Equal(t, 3, call.args["max_results"])
	require.InDelta(t, 0.5, call.args["min_score"], 0.001)
}

// A wing or room left blank must be left out of the call rather than sent as an empty.
func TestBlankFiltersAreOmittedRatherThanSentEmpty(t *testing.T) {
	contextEnabled(t)
	viper.Set("mempalace.context.wing", "   ")
	calls := stubMCP(t, `{"results":[]}`, nil)

	_, err := SearchContextIfEnabled(context.Background(), "a question")

	require.NoError(t, err)
	require.NotContains(t, (*calls)[0].args, "wing")
	require.NotContains(t, (*calls)[0].args, "room")
}

func TestTheResultCountFallsBackToOne(t *testing.T) {
	contextEnabled(t)
	viper.Set("mempalace.context.max_results", 0)
	calls := stubMCP(t, `{"results":[]}`, nil)

	_, err := SearchContextIfEnabled(context.Background(), "a question")

	require.NoError(t, err)
	require.Equal(t, 1, (*calls)[0].args["max_results"],
		"a missing limit is one memory, not every memory")
}

// This is the behaviour the whole feature rests on.
func TestAMemPalaceThatIsDownDoesNotStopTheQuestion(t *testing.T) {
	contextEnabled(t)
	stubMCP(t, "", errors.New("connection refused"))

	out, err := SearchContextIfEnabled(context.Background(), "a question")

	require.NoError(t, err, "a memory system that is unreachable is not a failed question")
	require.Empty(t, out)
}

func TestAnAnswerThatCannotBeDecodedIsTreatedAsNoContext(t *testing.T) {
	contextEnabled(t)
	stubMCP(t, `{not json`, nil)

	out, err := SearchContextIfEnabled(context.Background(), "a question")

	require.NoError(t, err)
	require.Empty(t, out)
}

// --- writing back ---------------------------------------------------------

func TestPersistingAnInvestigationRecordsTheGoalAndTheAnswer(t *testing.T) {
	calls := stubMCP(t, `{"ok":true}`, nil)

	require.NoError(t, PersistInvestigateResult(context.Background(),
		"find the slow query", "it is the missing index"))

	require.Len(t, *calls, 1)
	call := (*calls)[0]
	require.Equal(t, "mempalace_add_drawer", call.name)
	require.Equal(t, "gaia", call.args["wing"])
	require.Equal(t, "investigate", call.args["room"])
	require.Contains(t, call.args["content"], "goal: find the slow query")
	require.Contains(t, call.args["content"], "final_answer: it is the missing index")
}

func TestPersistingAToolExecutionRecordsItsExitCode(t *testing.T) {
	calls := stubMCP(t, `{"ok":true}`, nil)

	require.NoError(t, PersistToolExecution(context.Background(), "git status", "clean", 0))

	call := (*calls)[0]
	require.Equal(t, "tool", call.args["room"])
	require.Contains(t, call.args["content"], "command: git status")
	require.Contains(t, call.args["content"], "exit_code: 0")
}

func TestPersistingARoleDecisionRecordsWhichRoleAndWhy(t *testing.T) {
	calls := stubMCP(t, `{"ok":true}`, nil)

	require.NoError(t, PersistRoleDecision(context.Background(),
		"write me a commit message", "commit", "keyword match"))

	call := (*calls)[0]
	require.Equal(t, "roles", call.args["room"])
	require.Contains(t, call.args["content"], "selected_role: commit")
	require.Contains(t, call.args["content"], "reason: keyword match")
}

// Half a record is worse than none.
func TestNothingIncompleteIsEverWrittenBack(t *testing.T) {
	calls := stubMCP(t, `{"ok":true}`, nil)
	ctx := context.Background()

	require.NoError(t, PersistInvestigateResult(ctx, "a goal", "   "))
	require.NoError(t, PersistInvestigateResult(ctx, "  ", "an answer"))
	require.NoError(t, PersistToolExecution(ctx, "   ", "an outcome", 0))
	require.NoError(t, PersistRoleDecision(ctx, "some text", "   ", "a reason"))

	require.Empty(t, *calls)
}

// A write that fails is reported to the caller, unlike a read.
func TestAFailedWriteIsReportedRatherThanSwallowed(t *testing.T) {
	stubMCP(t, "", errors.New("connection refused"))

	err := PersistInvestigateResult(context.Background(), "a goal", "an answer")

	require.Error(t, err)
}

func TestTheDiaryIsOnlyWrittenWhenAskedFor(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	calls := stubMCP(t, `{"ok":true}`, nil)

	require.NoError(t, DiaryWriteIfEnabled(context.Background(), "a question", "an answer"))

	require.Empty(t, *calls, "the diary is off unless the config turns it on")
}

func TestTheDiaryRecordsBothSidesOfTheExchange(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("mempalace.diary.enabled", true)
	calls := stubMCP(t, `{"ok":true}`, nil)

	require.NoError(t, DiaryWriteIfEnabled(context.Background(), "a question", "an answer"))

	require.Len(t, *calls, 1)
	require.Equal(t, "mempalace_diary_write", (*calls)[0].name)
	require.Equal(t, "a question", (*calls)[0].args["query"])
	require.Equal(t, "an answer", (*calls)[0].args["response"])
}

// A turn with only one side is not an exchange.
func TestAHalfExchangeIsNotWrittenToTheDiary(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("mempalace.diary.enabled", true)
	calls := stubMCP(t, `{"ok":true}`, nil)
	ctx := context.Background()

	require.NoError(t, DiaryWriteIfEnabled(ctx, "a question", "   "))
	require.NoError(t, DiaryWriteIfEnabled(ctx, "  ", "an answer"))

	require.Empty(t, *calls)
}

// --- rendering what came back ---------------------------------------------

func TestMemoryContextListsEachItem(t *testing.T) {
	out := BuildMemoryContext([]MemoryItem{
		{Text: "the first memory"},
		{Text: "the second memory"},
	}, nil)

	require.Contains(t, out, "Memory Context:")
	require.Contains(t, out, "- the first memory")
	require.Contains(t, out, "- the second memory")
}

// With no structured items, the raw answer is shown rather than nothing.
func TestMemoryContextFallsBackToTheRawAnswer(t *testing.T) {
	out := BuildMemoryContext(nil, json.RawMessage(`{"unexpected":"shape"}`))

	require.Contains(t, out, "raw")
	require.Contains(t, out, "unexpected")
}

func TestMemoryContextOfNothingIsEmpty(t *testing.T) {
	require.Empty(t, BuildMemoryContext(nil, nil))
}

func TestFormattingItemsSkipsTheOnesWithNothingToSay(t *testing.T) {
	out := formatItems([]MemoryItem{
		{Text: "kept"},
		{Text: "   "},
		{Text: ""},
	}, nil)

	require.Equal(t, 1, strings.Count(out, "- "))
	require.Contains(t, out, "kept")
}
