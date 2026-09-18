package agent

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/plugins/ask"
)

// The loop's job is to stop.

// scriptedModel answers with a fixed sequence of turns, then repeats its last one.
func scriptedModel(turns ...ask.AskResponse) (func(context.Context, ask.AskRequest) (ask.AskResponse, error), *[]ask.AskRequest) {
	seen := &[]ask.AskRequest{}
	i := 0
	return func(_ context.Context, req ask.AskRequest) (ask.AskResponse, error) {
		*seen = append(*seen, req)
		if i < len(turns) {
			turn := turns[i]
			i++
			return turn, nil
		}
		return turns[len(turns)-1], nil
	}, seen
}

func answers(text string) ask.AskResponse {
	return ask.AskResponse{Text: text}
}

func asksFor(name string, args map[string]any) ask.AskResponse {
	return ask.AskResponse{ToolCalls: []ask.ToolCall{{Name: name, Arguments: args}}}
}

func runWith(t *testing.T, tools *Toolset, send func(context.Context, ask.AskRequest) (ask.AskResponse, error), maxSteps int) Result {
	t.Helper()
	result, err := Run(context.Background(), Options{
		Task: "do the thing", Send: send, Tools: tools, MaxSteps: maxSteps,
	})
	require.NoError(t, err)
	return result
}

func TestAModelThatAnswersEndsTheRun(t *testing.T) {
	ts, _ := aProject(t)
	send, _ := scriptedModel(answers("here is what I found"))

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopAnswered, result.StopReason)
	require.Equal(t, "here is what I found", result.Answer)
	require.Len(t, result.Steps, 1)
}

func TestAToolCallIsRunAndItsResultGoesBack(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600))
	send, seen := scriptedModel(
		asksFor("read_file", map[string]any{"path": "main.go"}),
		answers("it is package main"),
	)

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopAnswered, result.StopReason)
	require.Len(t, result.Steps, 2)
	require.Contains(t, result.Steps[0].Results[0], "package main")

	// The second turn must carry the tool result.
	second := (*seen)[1]
	require.Len(t, second.Messages, 4)
	require.Equal(t, "tool", second.Messages[2].Role)
	require.Equal(t, "read_file", second.Messages[2].ToolName)
}

// A file read puts hundreds of lines between a model and what it was asked for,
// and a small one answers about what it just read instead of doing the job.
func TestTheTaskIsRestatedAfterEveryToolResult(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600))
	send, seen := scriptedModel(
		asksFor("read_file", map[string]any{"path": "main.go"}),
		answers("done"),
	)

	runWith(t, ts, send, 10)

	last := (*seen)[1].Messages[3]
	require.Equal(t, "user", last.Role, "the reminder comes after the output, not before")
	require.Contains(t, last.Content, "do the thing", "it names the task again")
	require.Contains(t, last.Content, "Do not describe the code")
}

func TestTheToolsAreOfferedOnEveryTurn(t *testing.T) {
	ts, _ := aProject(t)
	send, seen := scriptedModel(
		asksFor("list_files", nil),
		answers("done"),
	)

	runWith(t, ts, send, 10)

	for i, req := range *seen {
		require.NotEmptyf(t, req.Tools, "turn %d offered no tools", i+1)
	}
}

// A model that asks for the same thing twice running is stuck, not working.
func TestTheSameCallTwiceRunningStopsTheRun(t *testing.T) {
	ts, _ := aProject(t)
	send, _ := scriptedModel(asksFor("list_files", map[string]any{"path": "."}))

	result := runWith(t, ts, send, 50)

	require.Equal(t, StopRepeating, result.StopReason)
	require.Len(t, result.Steps, 2, "the second identical turn is where it is noticed")
}

// Reading two different files is progress.
func TestTheSameToolWithDifferentArgumentsIsNotRepeating(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.go"), []byte("b"), 0o600))
	send, _ := scriptedModel(
		asksFor("read_file", map[string]any{"path": "a.go"}),
		asksFor("read_file", map[string]any{"path": "b.go"}),
		answers("read both"),
	)

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopAnswered, result.StopReason)
}

// A model that keeps working without ever finishing is the ordinary failure.
func TestARunThatNeverFinishesStopsAtTheCeiling(t *testing.T) {
	ts, root := aProject(t)
	for _, name := range []string{"a.go", "b.go", "c.go", "d.go"} {
		require.NoError(t, os.WriteFile(filepath.Join(root, name), []byte("x"), 0o600))
	}
	i := 0
	files := []string{"a.go", "b.go", "c.go", "d.go"}
	send := func(context.Context, ask.AskRequest) (ask.AskResponse, error) {
		call := asksFor("read_file", map[string]any{"path": files[i%len(files)]})
		i++
		return call, nil
	}

	result := runWith(t, ts, send, 3)

	require.Equal(t, StopMaxSteps, result.StopReason)
	require.Len(t, result.Steps, 3)
}

func TestTheCeilingHasADefault(t *testing.T) {
	require.Positive(t, DefaultMaxSteps)

	ts, _ := aProject(t)
	send, seen := scriptedModel(answers("done"))

	_, err := Run(context.Background(), Options{Task: "x", Send: send, Tools: ts})

	require.NoError(t, err)
	require.Len(t, *seen, 1, "a run with no ceiling configured still runs")
}

func TestAModelThatCannotBeReachedStopsTheRun(t *testing.T) {
	ts, _ := aProject(t)
	send := func(context.Context, ask.AskRequest) (ask.AskResponse, error) {
		return ask.AskResponse{}, errors.New("connection refused")
	}

	result, err := Run(context.Background(), Options{Task: "x", Send: send, Tools: ts})

	require.ErrorContains(t, err, "connection refused")
	require.Equal(t, StopProviderFailed, result.StopReason)
}

// Ctrl-C mid-run: the answer is what happened so far.
func TestACancelledRunStopsAndSaysSo(t *testing.T) {
	ts, _ := aProject(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	send, _ := scriptedModel(answers("never asked"))

	result, err := Run(ctx, Options{Task: "x", Send: send, Tools: ts})

	require.NoError(t, err)
	require.Equal(t, StopCancelled, result.StopReason)
	require.Empty(t, result.Steps)
}

func TestWhatWasWrittenIsReportedWhateverStoppedTheRun(t *testing.T) {
	ws, _ := aWorkspace(t)
	perms := DefaultPermissions()
	perms.AllowWrites = true
	ts := NewToolset(ws, perms)

	send, _ := scriptedModel(asksFor("write_file", map[string]any{"path": "new.txt", "content": "x"}))

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopRepeating, result.StopReason, "it wrote the same file twice and stopped")
	require.Equal(t, []string{"new.txt"}, result.FilesWritten)
}

// A tool that fails is a result the model reads and acts on, not the end of the run.
func TestAToolFailureIsAnObservationAndNotAnEnding(t *testing.T) {
	ts, _ := aProject(t)
	send, _ := scriptedModel(
		asksFor("read_file", map[string]any{"path": "nope.go"}),
		answers("that file is not there"),
	)

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopAnswered, result.StopReason)
	require.Contains(t, result.Steps[0].Results[0], "error:")
}

func TestSeveralCallsInOneTurnAllRun(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "a.go"), []byte("a"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "b.go"), []byte("b"), 0o600))

	send, _ := scriptedModel(
		ask.AskResponse{ToolCalls: []ask.ToolCall{
			{Name: "read_file", Arguments: map[string]any{"path": "a.go"}},
			{Name: "read_file", Arguments: map[string]any{"path": "b.go"}},
		}},
		answers("read both"),
	)

	result := runWith(t, ts, send, 10)

	require.Len(t, result.Steps[0].Results, 2)
}

// Somebody watching a run should see the work happen rather than a cursor.
func TestEachTurnIsObservedAsItHappens(t *testing.T) {
	ts, _ := aProject(t)
	send, _ := scriptedModel(asksFor("list_files", nil), answers("done"))

	var seen []Step
	_, err := Run(context.Background(), Options{
		Task: "x", Send: send, Tools: ts, MaxSteps: 10,
		Observe: func(step Step) { seen = append(seen, step) },
	})

	require.NoError(t, err)
	require.Len(t, seen, 2)
	require.Equal(t, 1, seen[0].Number)
	require.Equal(t, "list_files", seen[0].ToolCalls[0].Name)
}

func TestARunNeedsATaskAModelAndTools(t *testing.T) {
	ts, _ := aProject(t)
	send, _ := scriptedModel(answers("x"))

	_, err := Run(context.Background(), Options{Task: "  ", Send: send, Tools: ts})
	require.ErrorIs(t, err, ErrNoTask)

	_, err = Run(context.Background(), Options{Task: "x", Tools: ts})
	require.ErrorContains(t, err, "no model")

	_, err = Run(context.Background(), Options{Task: "x", Send: send})
	require.ErrorContains(t, err, "no tools")
}

func TestTheRoleReachesTheModel(t *testing.T) {
	ts, _ := aProject(t)
	send, seen := scriptedModel(answers("done"))

	_, err := Run(context.Background(), Options{
		Task: "x", Send: send, Tools: ts, SystemPrompt: "you are careful",
	})

	require.NoError(t, err)
	require.Equal(t, "you are careful", (*seen)[0].SystemPrompt)
}

// Two calls are the same call whatever order a Go map iterated its arguments.
func TestACallSignatureDoesNotDependOnMapOrder(t *testing.T) {
	first := callSignature([]ask.ToolCall{{Name: "t", Arguments: map[string]any{"a": "1", "b": "2", "c": "3"}}})
	for range 20 {
		require.Equal(t, first,
			callSignature([]ask.ToolCall{{Name: "t", Arguments: map[string]any{"c": "3", "b": "2", "a": "1"}}}))
	}
}

// A model that made three calls in one turn used to print the turn number three times.
func TestATurnWithSeveralCallsIsNumberedOnce(t *testing.T) {
	step := Step{
		Number: 4,
		ToolCalls: []ask.ToolCall{
			{Name: "write_file", Arguments: map[string]any{"path": "main.go"}},
			{Name: "run_command", Arguments: map[string]any{"command": "go test ./..."}},
		},
		Results: []string{"wrote main.go", "ok"},
	}

	out := renderStep(step)

	require.Equal(t, 1, strings.Count(out, "4."))
	require.Contains(t, out, "write_file")
	require.Contains(t, out, "run_command")
}

// A whole file read into the progress output buries the thing a person is watching for.
func TestALongResultIsShortenedInTheTrace(t *testing.T) {
	step := Step{
		Number:    1,
		ToolCalls: []ask.ToolCall{{Name: "read_file", Arguments: map[string]any{"path": "main.go"}}},
		Results:   []string{strings.Repeat("a line of a file\n", 100)},
	}

	out := renderStep(step)

	require.Less(t, len(out), 200)
	require.Contains(t, out, "read_file")
}

// Changing model per use is the point of having four commands rather than one setting.
func TestTheModelAndProviderTypedWinOverTheConfigured(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("agent.model", "configured-model")
	viper.Set("agent.provider", "ollama")

	cmd := agentCommand(t)
	require.NoError(t, cmd.Flags().Set("model", "typed-model"))
	require.NoError(t, cmd.Flags().Set("provider", "mistral"))

	req, err := NewAgentPlugin().request(cmd)

	require.NoError(t, err)
	require.Equal(t, "typed-model", req.Model)
	require.Equal(t, "mistral", req.Provider)
}

// With nothing typed, the agent's own key wins over the shared one.
func TestTheAgentKeyWinsOverTheSharedOne(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "shared-model")
	viper.Set("agent.model", "agent-model")

	req, err := NewAgentPlugin().request(agentCommand(t))

	require.NoError(t, err)
	require.Equal(t, "agent-model", req.Model)
}

func TestWithNoModelAnywhereTheAgentSaysWhatIsMissing(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	ask.NoModelsInstalled(t)

	_, err := NewAgentPlugin().request(agentCommand(t))

	require.ErrorContains(t, err, "no model configured")
}

// agentCommand carries the flags request() reads.
func agentCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("model", "", "")
	cmd.Flags().String("provider", "", "")
	return cmd
}

// Measured on a 7B: it narrated write_file as text and changed nothing.
func TestAModelThatNarratesItsCallsIsReportedAsSuch(t *testing.T) {
	ws, _ := aWorkspace(t)
	perms := DefaultPermissions()
	perms.AllowWrites = true
	ts := NewToolset(ws, perms)

	send, _ := scriptedModel(answers(
		"I will fix it.\n```go\nwrite_file(\"main.go\", \"package main\")\n```\nThen run the tests."))

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopToolsIgnored, result.StopReason)
	require.Empty(t, result.FilesWritten, "it described a write and performed none")
}

// A JSON call written as text used to stop the run; it is read back out and
// run instead, which is the difference between a wasted turn and a done task.
func TestAModelNarratingAJSONCallHasItRunForIt(t *testing.T) {
	ts, _ := aProject(t)
	send, _ := scriptedModel(
		answers(`{"name": "list_files", "arguments": {"path": "."}}`),
		answers("done"),
	)

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopAnswered, result.StopReason)
	require.True(t, result.Steps[0].Recovered)
	require.Equal(t, "list_files", result.Steps[0].ToolCalls[0].Name)
}

// Prose that merely mentions a tool is an ordinary answer.
func TestAnAnswerThatMentionsAToolInProseIsStillAnAnswer(t *testing.T) {
	ts, _ := aProject(t)
	send, _ := scriptedModel(answers(
		"You could read_file the config to check, but the bug is in the parser."))

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopAnswered, result.StopReason)
}

func TestAPlainAnswerIsNotMistakenForANarratedCall(t *testing.T) {
	ts, _ := aProject(t)
	send, _ := scriptedModel(answers("The function subtracts where it should add."))

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopAnswered, result.StopReason)
}

// A model that wrote the call instead of making it is one step from the answer,
// not finished.
func TestACallWrittenAsTextIsRunAnyway(t *testing.T) {
	ts, root := aWritingProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600))
	send, _ := scriptedModel(
		answers(`I will do this now:
{"name": "write_file", "arguments": {"path": "new.go", "content": "package new\n"}}`),
		answers("done"),
	)

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopAnswered, result.StopReason)
	require.Contains(t, result.FilesWritten, "new.go")
	require.True(t, result.Steps[0].Recovered, "and it is marked as recovered")
	require.FileExists(t, filepath.Join(root, "new.go"))
}

// Text that is not a call still ends the run, and still says why.
func TestProseAboutToolsStillEndsTheRun(t *testing.T) {
	ts, _ := aProject(t)
	send, _ := scriptedModel(answers(`I would use read_file("main.go") to look at it.`))

	result := runWith(t, ts, send, 10)

	require.Equal(t, StopToolsIgnored, result.StopReason)
	require.False(t, result.Steps[0].Recovered)
}
