package config

import (
	"bytes"
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	gaiaconfig "gaia/config"
	"gaia/kernel"
	"gaia/plugins/shared"
)

// `gaia config` from end to end, against a HOME of its own: the trust store and
// the configuration file both live under it, so nothing here touches the real one.

func configCommand(t *testing.T, name string) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	viper.Reset()
	t.Cleanup(viper.Reset)
	t.Setenv("HOME", t.TempDir())
	previous := gaiaconfig.CfgFile
	t.Cleanup(func() { gaiaconfig.CfgFile = previous })
	gaiaconfig.CfgFile = ""

	cmds, err := NewConfigPlugin().Register(kernel.NewKernel())
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
	t.Fatalf("there is no `config %s`", name)
	return nil, nil, nil
}

// aRepository makes a git repository, which is what the trust commands work on.
func aRepository(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	cmd := exec.Command("git", "init", "--quiet", dir)
	require.NoError(t, cmd.Run())
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)
	return resolved
}

func TestListShowsEveryKeyAndItsValue(t *testing.T) {
	cmd, out, _ := configCommand(t, "list")
	viper.Set("model", "a-model")
	viper.Set("agent.max_steps", 20)

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), "model: a-model")
	require.Contains(t, out.String(), "agent.max_steps: 20")
}

// A role prompt is pages long, and `config list` is for seeing what is set.
func TestShortCutsValuesTooLongToRead(t *testing.T) {
	cmd, out, _ := configCommand(t, "list")
	viper.Set("prompt", strings.Repeat("x", 200))
	require.NoError(t, cmd.Flags().Parse([]string{"--short"}))

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), strings.Repeat("x", 80)+"...")
	require.NotContains(t, out.String(), strings.Repeat("x", 81))
}

// A value that fits is shown whole: cutting it would be worse than useless.
func TestShortLeavesAValueThatFitsAlone(t *testing.T) {
	cmd, out, _ := configCommand(t, "list")
	viper.Set("prompt", strings.Repeat("x", 80))
	require.NoError(t, cmd.Flags().Parse([]string{"--short"}))

	require.NoError(t, cmd.RunE(cmd, nil))

	require.NotContains(t, out.String(), "...")
}

func TestGetShowsOneValue(t *testing.T) {
	cmd, out, _ := configCommand(t, "get")
	viper.Set("model", "a-model")

	require.NoError(t, cmd.RunE(cmd, []string{"model"}))

	require.Contains(t, out.String(), "a-model")
}

// A key nobody set is a typo, not an empty value.
func TestGetSaysSoWhenAKeyIsNotSet(t *testing.T) {
	cmd, _, errOut := configCommand(t, "get")

	err := cmd.RunE(cmd, []string{"nosuchkey"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.Contains(t, errOut.String(), "not set")
}

// A list key is shown as a list, so `config get` and `config set` speak the
// same language about it.
func TestGetShowsAListAsAList(t *testing.T) {
	cmd, out, _ := configCommand(t, "get")
	viper.Set("agent.allowlist", []string{"go", "make"})

	require.NoError(t, cmd.RunE(cmd, []string{"agent.allowlist"}))

	require.Contains(t, out.String(), `["go","make"]`)
}

func TestSetWritesTheValueToTheFile(t *testing.T) {
	cmd, out, _ := configCommand(t, "set")
	require.NoError(t, gaiaconfig.InitConfig())

	require.NoError(t, cmd.RunE(cmd, []string{"model", "a-model"}))

	require.Contains(t, out.String(), "Updated model")
	written, err := os.ReadFile(gaiaconfig.CfgFile)
	require.NoError(t, err)
	require.Contains(t, string(written), "a-model")
}

func TestCreateMakesTheFileAndPathSaysWhereItIs(t *testing.T) {
	cmd, out, _ := configCommand(t, "create")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), "config.yaml")
	require.FileExists(t, gaiaconfig.CfgFile)
}

func TestPathMakesTheFileIfNobodyHasYet(t *testing.T) {
	cmd, out, _ := configCommand(t, "path")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), "config.yaml")
	require.FileExists(t, gaiaconfig.CfgFile)
}

// Trust is what decides whether a repository's own .gaia.yaml is honoured, so
// these are the commands that change what gaia will read.

func TestTrustingARepositoryMakesItTrusted(t *testing.T) {
	cmd, out, _ := configCommand(t, "trust")
	repo := aRepository(t)

	require.NoError(t, cmd.RunE(cmd, []string{repo}))

	require.Contains(t, out.String(), repo)
	trusted, err := gaiaconfig.IsRepositoryTrusted(repo)
	require.NoError(t, err)
	require.True(t, trusted)
}

func TestUntrustingTakesItBack(t *testing.T) {
	repo := aRepository(t)
	cmd, _, _ := configCommand(t, "untrust")
	require.NoError(t, gaiaconfig.TrustRepository(repo))

	require.NoError(t, cmd.RunE(cmd, []string{repo}))

	trusted, err := gaiaconfig.IsRepositoryTrusted(repo)
	require.NoError(t, err)
	require.False(t, trusted)
}

func TestTrustedReportsTheStatusOfOneRepository(t *testing.T) {
	repo := aRepository(t)
	cmd, out, _ := configCommand(t, "trusted")

	require.NoError(t, cmd.RunE(cmd, []string{repo}))
	require.Contains(t, out.String(), "Trusted: no")

	require.NoError(t, gaiaconfig.TrustRepository(repo))
	out.Reset()
	require.NoError(t, cmd.RunE(cmd, []string{repo}))
	require.Contains(t, out.String(), "Trusted: yes")
}

func TestTrustedListsEveryTrustedRepository(t *testing.T) {
	cmd, out, _ := configCommand(t, "trusted")
	repo := aRepository(t)
	require.NoError(t, gaiaconfig.TrustRepository(repo))

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), repo)
}

// Nothing trusted is the default, and it has to read as an answer rather than
// as a command that did not work.
func TestTrustedSaysSoWhenNothingIsTrusted(t *testing.T) {
	cmd, out, _ := configCommand(t, "trusted")

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), "No trusted repositories")
}

// A path that is not there must not be trusted by accident.
func TestAPathThatIsNotThereIsRefusedByEveryTrustCommand(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "nope")
	for _, name := range []string{"trust", "untrust", "trusted"} {
		t.Run(name, func(t *testing.T) {
			cmd, _, _ := configCommand(t, name)

			err := cmd.RunE(cmd, []string{missing})

			require.ErrorContains(t, err, "resolve repository root")
		})
	}
}

// With no path given, the repository worked in is the one meant.
func TestWithNoPathTheCurrentDirectoryIsTheRepository(t *testing.T) {
	repo := aRepository(t)
	cmd, out, _ := configCommand(t, "trust")
	t.Chdir(repo)

	require.NoError(t, cmd.RunE(cmd, nil))

	require.Contains(t, out.String(), repo)
}

func TestThePluginDeclaresItselfAndIsAlwaysOn(t *testing.T) {
	p := NewConfigPlugin()

	require.Equal(t, "config", p.ID())
	require.True(t, p.DefaultEnabled())
	require.Empty(t, p.DependsOn())
}

// An allowlist is a list. Stored as a string it is read back as nothing, and
// the commands it was meant to permit are refused with no explanation.
func TestSettingAnAllowlistStoresAList(t *testing.T) {
	cmd, _, _ := configCommand(t, "set")
	require.NoError(t, gaiaconfig.InitConfig())
	require.NoError(t, gaiaconfig.RegisterPluginSchema("agent", []string{"agent.allowlist"}))

	require.NoError(t, cmd.RunE(cmd, []string{"agent.allowlist", `["go","make"]`}))

	require.Equal(t, []string{"go", "make"}, viper.GetStringSlice("agent.allowlist"))
}

func TestAnAllowlistThatIsNotAListIsRefused(t *testing.T) {
	cmd, _, _ := configCommand(t, "set")
	require.NoError(t, gaiaconfig.InitConfig())
	require.NoError(t, gaiaconfig.RegisterPluginSchema("agent", []string{"agent.allowlist"}))

	err := cmd.RunE(cmd, []string{"agent.allowlist", "go"})

	require.ErrorContains(t, err, "JSON array")
}
