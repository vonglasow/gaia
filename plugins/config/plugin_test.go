package config

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
	"gaia/plugins/shared"
)

// The acceptance scenarios in bdd/config.feature cover what a person sees when they.

// subcommand finds one of the registered subcommands by its first word.
func subcommand(t *testing.T, root *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("no %q subcommand under %q", name, root.Name())
	return nil
}

func registerConfig(t *testing.T) *cobra.Command {
	t.Helper()
	cmds, err := NewConfigPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	return cmds[0]
}

func TestThePluginDeclaresItselfToTheKernel(t *testing.T) {
	p := NewConfigPlugin()

	require.Equal(t, "config", p.ID())
	require.True(t, p.DefaultEnabled())
	require.Nil(t, p.DependsOn())
	require.Nil(t, p.MCPTools())
	require.Nil(t, p.ConfigSchema(),
		"the config plugin owns the config.* namespace through the kernel, not through a schema of its own")
}

// If a subcommand disappears.
func TestEverySubcommandIsRegistered(t *testing.T) {
	root := registerConfig(t)

	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}

	for _, want := range []string{"list", "get", "set", "create", "path", "trust", "untrust", "trusted"} {
		require.Truef(t, names[want], "subcommand %q is missing", want)
	}
}

func TestGetAndSetTakeExactlyTheArgumentsTheyNeed(t *testing.T) {
	root := registerConfig(t)

	require.Error(t, subcommand(t, root, "get").Args(nil, []string{}))
	require.NoError(t, subcommand(t, root, "get").Args(nil, []string{"ask.model"}))
	require.Error(t, subcommand(t, root, "get").Args(nil, []string{"ask.model", "extra"}))

	require.Error(t, subcommand(t, root, "set").Args(nil, []string{"ask.model"}))
	require.NoError(t, subcommand(t, root, "set").Args(nil, []string{"ask.model", "llama3.1"}))
}

func TestGetReadsAPlainKeyBack(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ask.model", "llama3.1")

	get := subcommand(t, registerConfig(t), "get")
	var out bytes.Buffer
	get.SetOut(&out)

	require.NoError(t, get.RunE(get, []string{"ask.model"}))
	require.Contains(t, out.String(), "llama3.1")
}

// A list key read as a plain value prints Go's own slice formatting.
func TestGetReadsAListKeyBackAsJSON(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("plugins.enabled", []string{"ask", "chat"})

	get := subcommand(t, registerConfig(t), "get")
	var out bytes.Buffer
	get.SetOut(&out)

	require.NoError(t, get.RunE(get, []string{"plugins.enabled"}))
	require.Contains(t, out.String(), `["ask","chat"]`)
}

func TestGetOnAKeyNobodySetFailsAndSaysWhy(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	get := subcommand(t, registerConfig(t), "get")
	var out, errOut bytes.Buffer
	get.SetOut(&out)
	get.SetErr(&errOut)

	err := get.RunE(get, []string{"ask.model"})

	require.ErrorIs(t, err, shared.ErrReported,
		"reading a key that is not set is a failure a script must be able to see")
	require.Contains(t, errOut.String(), "is not set")
	require.Empty(t, out.String(), "a reason is not a result, so it does not go to standard output")
}

func TestListPrintsEveryKeyItCanSee(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ask.model", "llama3.1")
	viper.Set("cache.enabled", true)

	list := subcommand(t, registerConfig(t), "list")
	var out bytes.Buffer
	list.SetOut(&out)
	require.NoError(t, list.Flags().Parse(nil))

	require.NoError(t, list.RunE(list, nil))
	require.Contains(t, out.String(), "ask.model")
	require.Contains(t, out.String(), "cache.enabled")
}

// --short exists because a role prompt is hundreds of characters and turns the listing.
func TestListShortensLongValuesWhenAsked(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	long := ""
	for len(long) < 200 {
		long += "prompt "
	}
	viper.Set("ask.role", long)

	list := subcommand(t, registerConfig(t), "list")
	var out bytes.Buffer
	list.SetOut(&out)
	require.NoError(t, list.Flags().Parse([]string{"--short"}))

	require.NoError(t, list.RunE(list, nil))
	require.Contains(t, out.String(), "...")
	require.NotContains(t, out.String(), long)
}

func TestSetRefusesAKeyNoPluginDeclared(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	set := subcommand(t, registerConfig(t), "set")
	var out bytes.Buffer
	set.SetOut(&out)

	err := set.RunE(set, []string{"bogus.key", "value"})

	require.ErrorContains(t, err, "invalid config key",
		"this is one of the few paths that returns an error rather than printing one, which is why the shell sees it")
}
