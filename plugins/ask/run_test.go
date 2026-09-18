package ask

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
	"gaia/plugins/shared"
)

// The command from end to end, with a model that answers whatever a test says.

type recordingProvider struct {
	answer string
	calls  []AskRequest
	err    error
}

func (r *recordingProvider) Name() string { return "ollama" }

func (r *recordingProvider) Send(_ context.Context, req AskRequest) (AskResponse, error) {
	r.calls = append(r.calls, req)
	if r.err != nil {
		return AskResponse{}, r.err
	}
	return AskResponse{Text: r.answer}, nil
}

func (r *recordingProvider) SendStream(ctx context.Context, req AskRequest, onChunk func(string)) (AskResponse, error) {
	if r.err == nil {
		onChunk(r.answer)
	}
	return r.Send(ctx, req)
}

// asking builds the command with a model that answers as told, and with the
// cache, MemPalace and the roles directory all pointed somewhere empty.
func asking(t *testing.T, provider Provider, flags ...string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Set("model", "a-model")
	viper.Set("roles.directory", filepath.Join(home, "roles"))

	p := NewAskPlugin()
	p.RegisterProvider(provider)
	cmds, err := p.Register(kernel.NewKernel())
	require.NoError(t, err)

	cmd := cmds[0]
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(""))
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Parse(flags))
	return cmd, &out, &errOut
}

func TestTheAnswerIsPrinted(t *testing.T) {
	cmd, out, _ := asking(t, &recordingProvider{answer: "42"})

	require.NoError(t, cmd.RunE(cmd, []string{"what", "is", "the", "answer"}))

	require.Contains(t, out.String(), "42")
}

func TestTheWordsTypedAreOneQuestion(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, _, _ := asking(t, provider)

	require.NoError(t, cmd.RunE(cmd, []string{"what", "is", "the", "answer"}))

	require.Len(t, provider.calls, 1)
	require.Equal(t, "what is the answer", provider.calls[0].Message)
}

func TestWithNothingToAskNothingIsAsked(t *testing.T) {
	provider := &recordingProvider{answer: "never asked"}
	cmd, _, errOut := asking(t, provider)

	err := cmd.RunE(cmd, nil)

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "No message provided")
	require.Empty(t, provider.calls)
}

func TestWithNoModelAnywhereNothingIsAsked(t *testing.T) {
	NoModelsInstalled(t)

	provider := &recordingProvider{answer: "never asked"}
	cmd, _, errOut := asking(t, provider)
	viper.Set("model", "")

	err := cmd.RunE(cmd, []string{"a question"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.NotEmpty(t, errOut.String())
	require.Empty(t, provider.calls)
}

func TestAModelThatCannotBeReachedIsReported(t *testing.T) {
	cmd, _, errOut := asking(t, &recordingProvider{err: errors.New("connection refused")})

	err := cmd.RunE(cmd, []string{"a question"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "is ollama running?")
}

// An empty answer is a failure: a shell reading the output would see nothing
// and a zero exit code.
func TestAnEmptyAnswerIsAFailure(t *testing.T) {
	cmd, _, errOut := asking(t, &recordingProvider{answer: ""})

	err := cmd.RunE(cmd, []string{"a question"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "empty response")
}

func TestTheModelTypedWinsOverTheConfiguredOne(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, _, _ := asking(t, provider, "--model", "typed")

	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))

	require.Equal(t, "typed", provider.calls[0].Model)
}

func TestPullIsPassedThroughToTheProvider(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, _, _ := asking(t, provider, "--pull")

	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))

	require.True(t, provider.calls[0].Pull)
}

// A provider nobody registered falls back to ollama rather than failing.
func TestAnUnknownProviderFallsBackToOllama(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, out, _ := asking(t, provider)
	viper.Set("ask.provider", "nosuchprovider")

	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))

	require.Contains(t, out.String(), "42")
}

// A role is what turns a general model into a specialised one, so the prompt it
// carries has to reach the provider.
func TestTheRolesPromptReachesTheModel(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, _, _ := asking(t, provider)
	writeRole(t, "physics", "You are a physicist.")
	viper.Set("ask.role", "physics")

	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))

	require.Contains(t, provider.calls[0].SystemPrompt, "You are a physicist.")
}

func TestARoleNobodyWroteIsReportedBeforeAnythingIsAsked(t *testing.T) {
	provider := &recordingProvider{answer: "never asked"}
	cmd, _, errOut := asking(t, provider)
	viper.Set("ask.role", "nosuchrole")

	err := cmd.RunE(cmd, []string{"a question"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), `no role called "nosuchrole"`)
	require.Empty(t, provider.calls)
}

// Auto-selection is what makes a role happen without anybody naming one.
func TestARoleIsPickedFromTheQuestionWhenAutoSelectIsOn(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, _, _ := asking(t, provider)
	writeRole(t, "physics", "You are a physicist.")
	viper.Set("roles.auto_select", true)
	viper.Set("roles.keywords.physics", []string{"entanglement"})

	require.NoError(t, cmd.RunE(cmd, []string{"explain entanglement"}))

	require.Contains(t, provider.calls[0].SystemPrompt, "You are a physicist.")
}

func TestWithNoRoleAndNoAutoSelectionThereIsNoSystemPrompt(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, _, _ := asking(t, provider)

	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))

	require.Empty(t, provider.calls[0].SystemPrompt)
}

// Cached or not, the same question twice gives the same answer; what changes is
// whether the model was asked a second time.
func TestAnAnsweredQuestionIsNotAskedTwice(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, _, _ := asking(t, provider)
	viper.Set("cache.enabled", true)
	viper.Set("cache.directory", t.TempDir())

	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))
	out := freshOutput(cmd)
	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))

	require.Len(t, provider.calls, 1, "the second answer came from the cache")
	require.Contains(t, out.String(), "42")
}

func TestNoCacheAsksAgain(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, _, _ := asking(t, provider, "--no-cache")
	viper.Set("cache.enabled", true)
	viper.Set("cache.directory", t.TempDir())

	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))
	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))

	require.Len(t, provider.calls, 2)
}

func freshOutput(cmd *cobra.Command) *bytes.Buffer {
	var out bytes.Buffer
	cmd.SetOut(&out)
	return &out
}

func writeRole(t *testing.T, name, prompt string) {
	t.Helper()
	dir := viper.GetString("roles.directory")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	body := "name: " + name + "\ndescription: a " + name + "\nsystem_prompt: |\n  " + prompt + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o600))
}

// A session is only worth caching as a whole conversation.
func TestConvertingTheHistoryForTheCacheDropsHalfEmptyTurns(t *testing.T) {
	out := ToCacheMessages([]ChatMessage{
		{Role: "user", Content: "a question"},
		{Role: "", Content: "no role"},
		{Role: "assistant", Content: "   "},
		{Role: "assistant", Content: "an answer"},
	})

	require.Len(t, out, 2)
	require.Equal(t, "user", out[0].Role)
	require.Equal(t, "a question", out[0].Content)
	require.Equal(t, "an answer", out[1].Content)
}

func TestConvertingAnEmptyHistoryGivesAnEmptyList(t *testing.T) {
	require.Empty(t, ToCacheMessages(nil))
}

// A role nobody wrote is a typo, and the next thing anyone asks is what they
// should have typed instead.
func TestARoleNobodyWroteNamesTheOnesThatExist(t *testing.T) {
	provider := &recordingProvider{answer: "never asked"}
	cmd, _, errOut := asking(t, provider)
	writeRole(t, "physics", "You are a physicist.")
	writeRole(t, "cook", "You are a cook.")
	viper.Set("ask.role", "physicist")

	err := cmd.RunE(cmd, []string{"a question"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "cook, physics")
	require.Empty(t, provider.calls)
}

// roles.default_role naming a role nobody wrote is a stale setting, not a
// question that must fail: nobody typed it for this question.
func TestARoleAutoSelectionGuessesWrongDoesNotStopTheQuestion(t *testing.T) {
	provider := &recordingProvider{answer: "42"}
	cmd, _, _ := asking(t, provider)
	viper.Set("roles.auto_select", true)
	viper.Set("roles.default_role", "default")

	require.NoError(t, cmd.RunE(cmd, []string{"a question"}))

	require.Len(t, provider.calls, 1)
	require.Empty(t, provider.calls[0].SystemPrompt)
}

// A role typed by hand is different: a typo there is worth stopping for.
func TestARoleAskedForByNameStillStopsTheQuestion(t *testing.T) {
	provider := &recordingProvider{answer: "never asked"}
	cmd, _, errOut := asking(t, provider)
	viper.Set("roles.auto_select", true)
	viper.Set("ask.role", "nosuchrole")

	err := cmd.RunE(cmd, []string{"a question"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), `no role called "nosuchrole"`)
	require.Empty(t, provider.calls)
}

// A transport failure is the one message a person reads when nothing worked, so
// each kind says what to do next rather than what the library called it.
func TestEachWayAModelFailsSaysSomethingDifferent(t *testing.T) {
	req := AskRequest{Model: "qwen2.5:14b", Host: "192.168.1.42", Port: 11434, Timeout: time.Minute}

	for _, given := range []struct{ raw, expect string }{
		{"connection refused", "is ollama running?"},
		{"dial tcp: lookup nas: no such host", "OLLAMA_HOST=0.0.0.0"},
		{"dial tcp 192.168.1.42:11434: i/o timeout", "firewall"},
		{`{"error":"prediction aborted, token repeat limit reached"}`, "stuck repeating itself"},
		{"context deadline exceeded", "did not answer within"},
	} {
		got := DescribeSendError(errors.New(given.raw), req)
		require.ErrorContains(t, got, given.expect, "for %q", given.raw)
	}
}

// Anything unrecognised is passed through rather than dressed up as a diagnosis.
func TestAnUnknownFailureIsNotGuessedAt(t *testing.T) {
	err := errors.New("something nobody anticipated")

	require.Equal(t, err, DescribeSendError(err, AskRequest{}))
	require.NoError(t, DescribeSendError(nil, AskRequest{}))
}
