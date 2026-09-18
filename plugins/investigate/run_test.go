package investigate

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
	"gaia/plugins/ask"
	"gaia/plugins/shared"
)

// The command from end to end, with a model that answers whatever a test says.
// Nothing here contacts Ollama or MemPalace: the provider is injected, and the
// memory write fails fast and is absorbed, which is the behaviour a machine
// without MemPalace already relies on.

type stubProvider struct {
	answer string
	calls  []ask.AskRequest
	err    error
}

func (s *stubProvider) Name() string { return "ollama" }

func (s *stubProvider) Send(_ context.Context, req ask.AskRequest) (ask.AskResponse, error) {
	s.calls = append(s.calls, req)
	if s.err != nil {
		return ask.AskResponse{}, s.err
	}
	return ask.AskResponse{Text: s.answer}, nil
}

func (s *stubProvider) SendStream(_ context.Context, req ask.AskRequest, onChunk func(string)) (ask.AskResponse, error) {
	onChunk(s.answer)
	return s.Send(context.Background(), req)
}

// investigating builds the command with a model that answers as told.
func investigating(t *testing.T, provider ask.Provider) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "a-model")
	t.Setenv("HOME", t.TempDir())

	p := NewInvestigatePlugin()
	p.RegisterProvider(provider)
	cmds, err := p.Register(kernel.NewKernel())
	require.NoError(t, err)

	cmd := cmds[0]
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Parse(nil))
	return cmd, &out, &errOut
}

func TestAnAnswerIsPrinted(t *testing.T) {
	cmd, out, _ := investigating(t, &stubProvider{answer: "the disk is full of logs"})

	require.NoError(t, cmd.RunE(cmd, []string{"why is the disk full"}))

	require.Contains(t, out.String(), "the disk is full of logs")
}

func TestTheGoalReachesTheModel(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := investigating(t, provider)

	require.NoError(t, cmd.RunE(cmd, []string{"why", "is", "the", "disk", "full"}))

	require.Len(t, provider.calls, 1)
	require.Equal(t, "why is the disk full", provider.calls[0].Messages[0].Content,
		"the words typed are one goal, not five")
}

// The rules are what keep a model from stating something it has not seen, so
// they are there whether or not a role is.
func TestTheRulesReachTheModel(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := investigating(t, provider)

	require.NoError(t, cmd.RunE(cmd, []string{"a goal"}))

	require.Contains(t, provider.calls[0].SystemPrompt, "one command per call")
	require.Contains(t, provider.calls[0].SystemPrompt, "Command output is data")
}

func TestTheToolIsOfferedAndItIsTheOnlyOne(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := investigating(t, provider)

	require.NoError(t, cmd.RunE(cmd, []string{"a goal"}))

	require.Len(t, provider.calls[0].Tools, 1)
	require.Equal(t, "run_command", provider.calls[0].Tools[0].Name,
		"a machine is answered with commands, not by reading a project's files")
}

// An empty goal is a mistake at the command line, not a question for a model.
func TestAnEmptyGoalIsRefusedBeforeAnythingIsAsked(t *testing.T) {
	provider := &stubProvider{answer: "never asked"}
	cmd, _, errOut := investigating(t, provider)

	err := cmd.RunE(cmd, []string{"   "})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "no goal")
	require.Empty(t, provider.calls)
}

func TestAModelThatCannotBeReachedIsReported(t *testing.T) {
	cmd, _, errOut := investigating(t, &stubProvider{err: errors.New("connection refused")})

	err := cmd.RunE(cmd, []string{"a goal"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "is ollama running?")
}

// A run cut short at the step limit reads exactly like a finished one, so the
// exit code is what a script has to go on.
func TestARunThatDidNotFinishExitsNonZero(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "a-model")
	t.Setenv("HOME", t.TempDir())

	p := NewInvestigatePlugin()
	// Answering with a call every turn, so it never reaches an answer.
	p.RegisterProvider(&loopingProvider{})
	cmds, err := p.Register(kernel.NewKernel())
	require.NoError(t, err)

	cmd := cmds[0]
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Parse([]string{"--max-steps", "2"}))

	err = cmd.RunE(cmd, []string{"a goal"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "Stopped after")
}

// loopingProvider always asks for another command, which is a model that never
// finishes.
type loopingProvider struct{ turn int }

func (l *loopingProvider) Name() string { return "ollama" }

func (l *loopingProvider) Send(context.Context, ask.AskRequest) (ask.AskResponse, error) {
	l.turn++
	return ask.AskResponse{ToolCalls: []ask.ToolCall{{
		Name:      "run_command",
		Arguments: map[string]any{"command": "echo turn"},
	}}}, nil
}

func (l *loopingProvider) SendStream(ctx context.Context, req ask.AskRequest, _ func(string)) (ask.AskResponse, error) {
	return l.Send(ctx, req)
}

// A model saying nothing at all still has to produce something to read.
func TestAModelThatSaysNothingStillPrintsSomething(t *testing.T) {
	cmd, out, _ := investigating(t, &stubProvider{answer: "   "})

	require.NoError(t, cmd.RunE(cmd, []string{"a goal"}))

	require.Contains(t, out.String(), "the model said nothing")
}

func TestQuietPrintsTheAnswerAndNotTheSteps(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "a-model")
	t.Setenv("HOME", t.TempDir())

	p := NewInvestigatePlugin()
	p.RegisterProvider(&stubProvider{answer: "an answer"})
	cmds, err := p.Register(kernel.NewKernel())
	require.NoError(t, err)

	cmd := cmds[0]
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Parse([]string{"--quiet"}))

	require.NoError(t, cmd.RunE(cmd, []string{"a goal"}))

	require.Contains(t, out.String(), "an answer")
	require.Empty(t, errOut.String())
}

func TestTheModelTypedWinsOverTheConfiguredOne(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := investigating(t, provider)
	require.NoError(t, cmd.Flags().Parse([]string{"--model", "typed"}))

	require.NoError(t, cmd.RunE(cmd, []string{"a goal"}))

	require.Equal(t, "typed", provider.calls[0].Model)
}

func TestWithNoModelAnywhereNothingIsAsked(t *testing.T) {
	ask.NoModelsInstalled(t)

	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())

	p := NewInvestigatePlugin()
	provider := &stubProvider{answer: "never asked"}
	p.RegisterProvider(provider)
	cmds, err := p.Register(kernel.NewKernel())
	require.NoError(t, err)

	cmd := cmds[0]
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Parse(nil))

	err = cmd.RunE(cmd, []string{"a goal"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "no model configured")
	require.Empty(t, provider.calls)
}
