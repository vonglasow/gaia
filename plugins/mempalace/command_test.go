package mempalace

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
	"gaia/plugins/shared"
)

// `gaia mem` from end to end, over a stubbed transport: the MCP server is a
// separate process, and none of these tests start one.

// memCommand returns one subcommand of `mem`, wired to buffers.
func memCommand(t *testing.T, name string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)

	cmds, err := NewMemPalacePlugin().Register(kernel.NewKernel())
	require.NoError(t, err)

	var out, errOut bytes.Buffer
	for _, sub := range cmds[0].Commands() {
		if sub.Name() == name {
			sub.SetOut(&out)
			sub.SetErr(&errOut)
			sub.SetContext(context.Background())
			return sub, &out, &errOut
		}
	}
	t.Fatalf("there is no `mem %s`", name)
	return nil, nil, nil
}

// stubListTools replaces the transport for the one command that lists tools.
func stubListTools(t *testing.T, answer string, failWith error) {
	t.Helper()
	previous := listToolsFn
	listToolsFn = func(context.Context) (json.RawMessage, error) {
		if failWith != nil {
			return nil, failWith
		}
		return json.RawMessage(answer), nil
	}
	t.Cleanup(func() { listToolsFn = previous })
}

func TestStatusShowsWhatMemPalaceReported(t *testing.T) {
	cmd, out, _ := memCommand(t, "status")
	stubMCP(t, `{"total_drawers":12}`, nil)

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), "12")
}

// MemPalace is optional, so a machine without it has to say so and not crash.
func TestAMemPalaceThatIsDownIsReportedAndExitsNonZero(t *testing.T) {
	cmd, _, errOut := memCommand(t, "status")
	stubMCP(t, "", errors.New("mempalace is not installed"))

	err := cmd.RunE(cmd, nil)

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "mempalace is not installed")
}

func TestToolsListsWhatTheServerOffers(t *testing.T) {
	cmd, out, _ := memCommand(t, "tools")
	stubListTools(t, `{"tools":[{"name":"mempalace_search"}]}`, nil)

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), "mempalace_search")
}

func TestToolsReportsAServerItCannotReach(t *testing.T) {
	cmd, _, errOut := memCommand(t, "tools")
	stubListTools(t, "", errors.New("no such command"))

	err := cmd.RunE(cmd, nil)

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "no such command")
}

// `mem call` is the escape hatch: any tool the server has, by name.
func TestCallPassesTheToolNameAndItsArgumentsThrough(t *testing.T) {
	cmd, out, _ := memCommand(t, "call")
	calls := stubMCP(t, `{"ok":true}`, nil)

	require.NoError(t, cmd.RunE(cmd, []string{"mempalace_graph_stats", `{"wing":"gaia"}`}))

	require.Len(t, *calls, 1)
	require.Equal(t, "mempalace_graph_stats", (*calls)[0].name)
	require.Equal(t, "gaia", (*calls)[0].args["wing"])
	require.Contains(t, out.String(), "true")
}

func TestCallWithoutArgumentsSendsNone(t *testing.T) {
	cmd, _, _ := memCommand(t, "call")
	calls := stubMCP(t, `{}`, nil)

	require.NoError(t, cmd.RunE(cmd, []string{"mempalace_status"}))

	require.Nil(t, (*calls)[0].args)
}

// Arguments that are not JSON are a typo at the command line, caught before
// anything is sent.
func TestCallRefusesArgumentsThatAreNotJSON(t *testing.T) {
	cmd, _, errOut := memCommand(t, "call")
	calls := stubMCP(t, `{}`, nil)

	err := cmd.RunE(cmd, []string{"mempalace_status", "{not json"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "invalid json args")
	require.Empty(t, *calls)
}

func TestSearchShowsWhatWasFound(t *testing.T) {
	cmd, out, _ := memCommand(t, "search")
	stubMCP(t, `{"results":[{"content":"the deploy runs on Friday"}]}`, nil)

	require.NoError(t, cmd.RunE(cmd, []string{"deploy"}))

	require.Contains(t, out.String(), "the deploy runs on Friday")
}

func TestSearchSaysSoRatherThanPrintingAnEmptyBox(t *testing.T) {
	cmd, out, _ := memCommand(t, "search")
	stubMCP(t, `{"results":[]}`, nil)

	require.NoError(t, cmd.RunE(cmd, []string{"nothing about this"}))

	require.Contains(t, out.String(), "No results")
}

func TestSearchReportsAServerItCannotReach(t *testing.T) {
	cmd, _, errOut := memCommand(t, "search")
	stubMCP(t, "", errors.New("mempalace is not installed"))

	err := cmd.RunE(cmd, []string{"deploy"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "mempalace is not installed")
}

func TestAnEmptyQueryIsRefusedBeforeAnythingIsSent(t *testing.T) {
	for _, name := range []string{"search", "inject"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, errOut := memCommand(t, name)
			calls := stubMCP(t, `{}`, nil)

			err := cmd.RunE(cmd, []string{"   "})

			require.ErrorIs(t, err, shared.ErrReported)
			require.Contains(t, errOut.String(), "query is required")
			require.Empty(t, *calls)
		})
	}
}

// The flags are what a person reaches for when the defaults return too much.
func TestTheLimitsTypedWinOverTheConfiguredOnes(t *testing.T) {
	cmd, _, _ := memCommand(t, "search")
	viper.Set("mempalace.inject.max_results", 5)
	viper.Set("mempalace.inject.min_score", 0.1)
	calls := stubMCP(t, `{"results":[]}`, nil)
	require.NoError(t, cmd.Flags().Parse([]string{"--max-results", "2", "--min-score", "0.9"}))

	require.NoError(t, cmd.RunE(cmd, []string{"deploy"}))

	require.Equal(t, 2, (*calls)[0].args["max_results"])
	require.InDelta(t, 0.9, (*calls)[0].args["min_score"], 0.001)
}

func TestWithNoLimitsAnywhereFiveResultsAreAskedFor(t *testing.T) {
	cmd, _, _ := memCommand(t, "search")
	calls := stubMCP(t, `{"results":[]}`, nil)

	require.NoError(t, cmd.RunE(cmd, []string{"deploy"}))

	require.Equal(t, 5, (*calls)[0].args["max_results"])
}

func TestInjectRendersTheResultsAsContextForAModel(t *testing.T) {
	cmd, out, _ := memCommand(t, "inject")
	stubMCP(t, `{"results":[{"content":"the deploy runs on Friday"}]}`, nil)

	require.NoError(t, cmd.RunE(cmd, []string{"deploy"}))

	require.Contains(t, out.String(), "the deploy runs on Friday")
}

// An envelope nothing could be read out of is shown as it arrived, rather than
// silently becoming an empty context.
func TestInjectShowsAnAnswerItCouldNotReadRatherThanNothing(t *testing.T) {
	cmd, out, _ := memCommand(t, "inject")
	stubMCP(t, `{"unexpected":"shape"}`, nil)

	require.NoError(t, cmd.RunE(cmd, []string{"deploy"}))

	require.Contains(t, out.String(), "raw")
	require.Contains(t, out.String(), "unexpected")
}

func TestInjectReportsAServerItCannotReach(t *testing.T) {
	cmd, _, errOut := memCommand(t, "inject")
	stubMCP(t, "", errors.New("mempalace is not installed"))

	err := cmd.RunE(cmd, []string{"deploy"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "mempalace is not installed")
}

// Injection is what puts memory into a question asked elsewhere, and it is off
// until somebody turns it on.
func TestNothingIsInjectedWhileTheFeatureIsOff(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	calls := stubMCP(t, `{"results":[{"content":"anything"}]}`, nil)

	got, err := InjectIfEnabled(context.Background(), "deploy")

	require.NoError(t, err)
	require.Empty(t, got)
	require.Empty(t, *calls)
}

func TestAnEmptyQuestionInjectsNothing(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("mempalace.inject.enabled", true)
	calls := stubMCP(t, `{"results":[{"content":"anything"}]}`, nil)

	got, err := InjectIfEnabled(context.Background(), "   ")

	require.NoError(t, err)
	require.Empty(t, got)
	require.Empty(t, *calls)
}

func TestWhatIsInjectedIsWhatWasFound(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("mempalace.inject.enabled", true)
	stubMCP(t, `{"results":[{"content":"the deploy runs on Friday"}]}`, nil)

	got, err := InjectIfEnabled(context.Background(), "deploy")

	require.NoError(t, err)
	require.Contains(t, got, "the deploy runs on Friday")
}

func TestAMemPalaceThatIsDownStopsTheQuestionItWasAskedFor(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("mempalace.inject.enabled", true)
	stubMCP(t, "", errors.New("mempalace is not installed"))

	_, err := InjectIfEnabled(context.Background(), "deploy")

	require.ErrorContains(t, err, "mempalace is not installed")
}

// A search that fails with filters is tried once more without them, so a server
// that does not understand max_results still answers.
func TestASearchRefusedWithFiltersIsTriedAgainWithoutThem(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	attempts := 0
	previous := callToolFn
	callToolFn = func(_ context.Context, _ string, args map[string]interface{}) (json.RawMessage, error) {
		attempts++
		if _, filtered := args["max_results"]; filtered {
			return nil, errors.New("unexpected argument")
		}
		return json.RawMessage(`{"results":[{"content":"found anyway"}]}`), nil
	}
	t.Cleanup(func() { callToolFn = previous })

	items, _, err := searchMemories(context.Background(), "deploy", 5, 0)

	require.NoError(t, err)
	require.Equal(t, 2, attempts)
	require.Len(t, items, 1)
}

// A score below the threshold is not worth putting in front of a model.
func TestResultsBelowTheThresholdAreDropped(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	stubMCP(t, `{"results":[{"content":"close enough","score":0.9},{"content":"barely related","score":0.1}]}`, nil)

	items, _, err := searchMemories(context.Background(), "deploy", 0, 0.5)

	require.NoError(t, err)
	require.Len(t, items, 1)
	require.Contains(t, items[0].Text, "close enough")
}

func TestMemoryIsAddedToASystemPromptWithoutLosingIt(t *testing.T) {
	require.Equal(t, "be brief\n\nremembered", AppendMemory("be brief", "remembered"))
	require.Equal(t, "remembered", AppendMemory("", "remembered"))
	require.Equal(t, "be brief", AppendMemory("be brief", "   "))
}

func TestThePluginDeclaresItselfAndItsKeys(t *testing.T) {
	p := NewMemPalacePlugin()

	require.Equal(t, "mempalace", p.ID())
	require.True(t, p.DefaultEnabled())
	require.Empty(t, p.DependsOn())
	require.Empty(t, p.MCPTools(), "memory is reached through gaia, not offered over MCP")
	require.Contains(t, p.ConfigSchema(), "mempalace.inject.enabled")
}

// An answer understood to hold nothing is an answer. Handing a model
// {"results": []} as its memory context is noise it has to reason past.

func TestNothingFoundIsNoContextRatherThanRawJSON(t *testing.T) {
	for _, empty := range []string{
		`{"results":[]}`,
		`{"matches":[]}`,
		`[]`,
		`{"result":[]}`,
	} {
		require.Empty(t, BuildMemoryContext(nil, json.RawMessage(empty)), "for %s", empty)
	}
}

// An envelope nobody could read is worth showing as it arrived: it is the only
// clue to what the far end actually said.
func TestAnAnswerNobodyCouldReadIsStillShown(t *testing.T) {
	for _, unreadable := range []string{
		`{"unexpected":"shape"}`,
		`{"error":"the palace is locked"}`,
		`not json at all`,
	} {
		got := BuildMemoryContext(nil, json.RawMessage(unreadable))
		require.Contains(t, got, "raw", "for %s", unreadable)
	}
}

// Everything filtered out by min_score is also nothing found.
func TestResultsAllBelowTheThresholdAreNoContextEither(t *testing.T) {
	raw := json.RawMessage(`{"results":[{"content":"barely related","score":0.1}]}`)

	require.Empty(t, BuildMemoryContext(nil, raw))
}

func TestAnEmptyAnswerIsStillNoContext(t *testing.T) {
	require.Empty(t, BuildMemoryContext(nil, nil))
}

// `gaia mem inject` is where a person sees this, and it must read as an answer
// rather than as a command that did not work.
func TestInjectSaysSoWhenThereIsNothingToInject(t *testing.T) {
	cmd, out, _ := memCommand(t, "inject")
	stubMCP(t, `{"results":[]}`, nil)

	require.NoError(t, cmd.RunE(cmd, []string{"deploy"}))

	require.Contains(t, out.String(), "No results")
	require.NotContains(t, out.String(), "results\": []")
}

// And nothing is smuggled into a question asked elsewhere.
func TestNothingFoundInjectsNothingIntoAQuestion(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("mempalace.inject.enabled", true)
	stubMCP(t, `{"results":[]}`, nil)

	got, err := InjectIfEnabled(context.Background(), "deploy")

	require.NoError(t, err)
	require.Empty(t, got)
}
