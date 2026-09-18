package chat

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
	"gaia/plugins/ask"
	"gaia/plugins/shared"
)

// Nothing here contacts a model.

func TestThePluginDeclaresItselfToTheKernel(t *testing.T) {
	p := NewChatPlugin()

	require.Equal(t, "chat", p.ID())
	require.True(t, p.DefaultEnabled(), "a specialised conversation is nothing else covers")
	require.Nil(t, p.DependsOn())
	require.Nil(t, p.MCPTools())

	for _, key := range p.ConfigSchema() {
		require.True(t, strings.HasPrefix(key, "chat."),
			"%q is outside the plugin's namespace", key)
	}
}

func TestTheBuiltInProvidersAreRegistered(t *testing.T) {
	p := NewChatPlugin()

	for _, name := range []string{"ollama", "openai", "mistral"} {
		_, ok := p.providers[name]
		require.Truef(t, ok, "provider %q is not registered", name)
	}
}

// A nil provider is what a constructor returns when it gives up.
func TestRegisteringANilProviderIsIgnored(t *testing.T) {
	p := NewChatPlugin()
	before := len(p.providers)

	p.RegisterProvider(nil)

	require.Len(t, p.providers, before)
}

func TestRegisterReturnsTheChatCommandWithItsFlags(t *testing.T) {
	cmds, err := NewChatPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	cmd := cmds[0]
	require.Equal(t, "chat", cmd.Use)
	for _, flag := range []string{"host", "port", "model", "timeout", "no-cache", "refresh-cache"} {
		require.NotNilf(t, cmd.Flags().Lookup(flag), "flag --%s is missing", flag)
	}
}

func TestAnIncompleteConfigurationStopsTheSessionBeforeAnyProviderIsCalled(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	ask.NoModelsInstalled(t)

	cmds, err := NewChatPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)

	var out, errOut bytes.Buffer
	cmd := cmds[0]
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	require.NoError(t, cmd.Flags().Parse(nil))

	err = cmd.RunE(cmd, nil)

	require.ErrorIs(t, err, shared.ErrReported,
		"a session that could not open is a failure, not a quiet exit 0")
	require.Contains(t, errOut.String(), "no model configured")
	require.NotContains(t, out.String(), "Starting chat session",
		"no session is opened against a configuration that cannot reach anything")
}

// The session ends on the word a person types, not on end-of-input alone.
func TestTypingExitEndsTheSession(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("chat.provider", "ollama")
	viper.Set("chat.host", "localhost")
	viper.Set("chat.port", 11434)
	viper.Set("chat.model", "llama3.1")
	viper.Set("chat.timeout_seconds", 5)

	cmds, err := NewChatPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)

	var out, errOut bytes.Buffer
	cmd := cmds[0]
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("exit\n"))
	require.NoError(t, cmd.Flags().Parse(nil))

	require.NoError(t, cmd.RunE(cmd, nil))
	require.Contains(t, out.String(), "Starting chat session")
}

// A provider nobody registered falls back to ollama rather than failing.
func TestAnUnknownProviderFallsBackToOllama(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("chat.provider", "not-a-provider")
	viper.Set("chat.host", "localhost")
	viper.Set("chat.port", 11434)
	viper.Set("chat.model", "llama3.1")
	viper.Set("chat.timeout_seconds", 5)

	cmds, err := NewChatPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)

	var out, errOut bytes.Buffer
	cmd := cmds[0]
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("exit\n"))
	require.NoError(t, cmd.Flags().Parse(nil))

	require.NoError(t, cmd.RunE(cmd, nil))
	require.Contains(t, out.String(), "Starting chat session",
		"the session opens against ollama rather than refusing the unknown name")
	require.NotContains(t, errOut.String(), "Unknown provider")
}

// The provider is inferred from the model when none was named.
func TestTheProviderIsInferredFromTheModelWhenUnset(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("chat.host", "localhost")
	viper.Set("chat.port", 11434)
	viper.Set("chat.model", "llama3.1")
	viper.Set("chat.timeout_seconds", 5)

	cmds, err := NewChatPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)

	var out, errOut bytes.Buffer
	cmd := cmds[0]
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetIn(strings.NewReader("exit\n"))
	require.NoError(t, cmd.Flags().Parse(nil))

	require.NoError(t, cmd.RunE(cmd, nil))
	require.Contains(t, out.String(), "Starting chat session")
	require.NotContains(t, errOut.String(), "missing chat configuration",
		"an unset provider is filled in from the model name, not reported as missing")
}
