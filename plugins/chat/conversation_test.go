package chat

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
	"gaia/plugins/ask"
	"gaia/plugins/shared"
)

// The session from end to end, against a model that answers whatever a test
// says. Input is a string, so a whole conversation is one test.

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

// chatting types the given lines into a session and reports what came out.
func chatting(t *testing.T, provider ask.Provider, typed string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "a-model")
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Set("roles.directory", filepath.Join(home, "roles"))

	p := NewChatPlugin()
	p.RegisterProvider(provider)
	cmds, err := p.Register(kernel.NewKernel())
	require.NoError(t, err)

	cmd := cmds[0]
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader(typed))
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Parse(nil))
	return cmd, &out, &errOut
}

func TestAQuestionIsAnsweredAndTheAnswerIsShown(t *testing.T) {
	cmd, out, _ := chatting(t, &stubProvider{answer: "a superposition is…"}, "what is superposition\nexit\n")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), "a superposition is…")
}

// The second question carries the first, which is what makes this a
// conversation rather than two questions.
func TestTheConversationIsCarriedFromTurnToTurn(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := chatting(t, provider, "first\nsecond\nexit\n")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Len(t, provider.calls, 2)
	require.Len(t, provider.calls[1].Messages, 3, "user, assistant, user")
	require.Equal(t, "first", provider.calls[1].Messages[0].Content)
}

func TestResetForgetsTheConversation(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := chatting(t, provider, "first\n/reset\nsecond\nexit\n")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Len(t, provider.calls, 2)
	require.Len(t, provider.calls[1].Messages, 1, "the second question starts fresh")
}

// A command never reaches the model: it is addressed to gaia.
func TestACommandIsNotAQuestion(t *testing.T) {
	provider := &stubProvider{answer: "never asked"}
	cmd, out, _ := chatting(t, provider, "/help\nexit\n")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Empty(t, provider.calls)
	require.Contains(t, out.String(), "/role")
}

func TestAnEmptyLineIsNotAQuestionEither(t *testing.T) {
	provider := &stubProvider{answer: "never asked"}
	cmd, _, _ := chatting(t, provider, "\n\n   \nexit\n")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Empty(t, provider.calls)
}

// A piped session that runs out of lines ends, rather than spinning.
func TestRunningOutOfInputEndsTheSession(t *testing.T) {
	cmd, out, _ := chatting(t, &stubProvider{answer: "an answer"}, "a question\n")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), "EOF")
}

func TestAModelThatCannotBeReachedIsReportedAndTheSessionGoesOn(t *testing.T) {
	cmd, _, errOut := chatting(t, &stubProvider{err: errors.New("connection refused")},
		"a question\nexit\n")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, errOut.String(), "is ollama running?")
}

func TestWithNoModelAnywhereNothingIsAsked(t *testing.T) {
	ask.NoModelsInstalled(t)

	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())

	p := NewChatPlugin()
	provider := &stubProvider{answer: "never asked"}
	p.RegisterProvider(provider)
	cmds, err := p.Register(kernel.NewKernel())
	require.NoError(t, err)

	cmd := cmds[0]
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("a question\n"))
	cmd.SetContext(context.Background())
	require.NoError(t, cmd.Flags().Parse(nil))

	err = cmd.RunE(cmd, nil)

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "no model configured")
	require.Empty(t, provider.calls)
}

// Switching specialisation is the reason chat exists: the new role has to reach
// the model, and the conversation has to survive the switch.
func TestTheRoleChosenInSessionReachesTheModel(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := chatting(t, provider, "/role physics\na question\nexit\n")
	writeRole(t, "physics", "You are a physicist.")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Len(t, provider.calls, 1)
	require.Contains(t, provider.calls[0].SystemPrompt, "You are a physicist.")
}

func TestSwitchingRoleCarriesTheConversationIntoTheNewOne(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := chatting(t, provider, "first\n/role physics\nsecond\nexit\n")
	writeRole(t, "physics", "You are a physicist.")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Len(t, provider.calls, 2)
	require.Empty(t, provider.calls[0].SystemPrompt, "no role was in force yet")
	require.Contains(t, provider.calls[1].SystemPrompt, "You are a physicist.")
	require.Len(t, provider.calls[1].Messages, 3, "the earlier turns came along")
}

func TestTakingTheRoleOffGoesBackToAskingWithoutOne(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := chatting(t, provider, "/role physics\nfirst\n/role none\nsecond\nexit\n")
	writeRole(t, "physics", "You are a physicist.")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Len(t, provider.calls, 2)
	require.Contains(t, provider.calls[0].SystemPrompt, "You are a physicist.")
	require.Empty(t, provider.calls[1].SystemPrompt)
}

// Nobody named a role, so one is picked from what was asked.
func TestARoleIsPickedFromTheQuestionWhenAutoSelectIsOn(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, _, _ := chatting(t, provider, "explain entanglement\nexit\n")
	writeRole(t, "physics", "You are a physicist.")
	viper.Set("roles.auto_select", true)
	viper.Set("roles.keywords.physics", []string{"entanglement"})

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, provider.calls[0].SystemPrompt, "You are a physicist.")
}

// The same question twice in one session is answered from the cache, and the
// answer still joins the conversation.
func TestAnAnsweredQuestionIsNotAskedTwice(t *testing.T) {
	provider := &stubProvider{answer: "an answer"}
	cmd, out, _ := chatting(t, provider, "a question\n/reset\na question\nexit\n")
	viper.Set("cache.enabled", true)
	viper.Set("cache.directory", t.TempDir())

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Len(t, provider.calls, 1, "the second answer came from the cache")
	require.Equal(t, 2, strings.Count(out.String(), "an answer"))
}

func writeRole(t *testing.T, name, prompt string) {
	t.Helper()
	dir := viper.GetString("roles.directory")
	require.NoError(t, os.MkdirAll(dir, 0o750))
	body := "name: " + name + "\ndescription: a " + name + "\nsystem_prompt: |\n  " + prompt + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".yaml"), []byte(body), 0o600))
}
