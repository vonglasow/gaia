package kernel_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
)

// Booting: what happens before cobra sees anything.

// The config flag is read out of the raw arguments, because the file it names
// has to be loaded before the flag it came from can be parsed.
func TestTheConfigFlagIsFoundInEveryFormItIsWritten(t *testing.T) {
	for _, given := range [][]string{
		{"--config", "/a/path.yaml"},
		{"-c", "/a/path.yaml"},
		{"--config=/a/path.yaml"},
		{"-c=/a/path.yaml"},
		{"ask", "a question", "--config", "/a/path.yaml"},
	} {
		require.Equal(t, "/a/path.yaml", kernel.DetectConfigPath(given), "%v", given)
	}
}

func TestWithNoConfigFlagThereIsNoPath(t *testing.T) {
	require.Empty(t, kernel.DetectConfigPath(nil))
	require.Empty(t, kernel.DetectConfigPath([]string{"ask", "a question"}))
	require.Empty(t, kernel.DetectConfigPath([]string{"--configuration", "/a/path.yaml"}))
}

// `gaia --config` with nothing after it is a mistake, and must not read the
// next flag as a filename or walk off the end of the arguments.
func TestAConfigFlagWithNothingAfterItIsIgnored(t *testing.T) {
	require.Empty(t, kernel.DetectConfigPath([]string{"--config"}))
	require.Empty(t, kernel.DetectConfigPath([]string{"-c"}))
}

// The first one wins, the way a flag does.
func TestTheFirstConfigFlagIsTheOneUsed(t *testing.T) {
	require.Equal(t, "/first.yaml",
		kernel.DetectConfigPath([]string{"--config", "/first.yaml", "--config", "/second.yaml"}))
}

func TestARegisteredPluginCanBeFoundByID(t *testing.T) {
	k := kernel.NewKernel()
	require.NoError(t, k.RegisterPlugin(&testPlugin{id: "one", def: true}))

	found, ok := k.Plugin("one")
	require.True(t, ok)
	require.Equal(t, "one", found.ID())

	_, ok = k.Plugin("nosuchplugin")
	require.False(t, ok)
}

// Listings are sorted: two runs of `gaia plugins list` that differ would read
// as something having changed.
func TestPluginsAreListedInAStableOrder(t *testing.T) {
	resetViper()
	defer resetViper()
	k := kernel.NewKernel()
	for _, id := range []string{"zeta", "alpha", "middle"} {
		require.NoError(t, k.RegisterPlugin(&testPlugin{id: id, def: true}))
	}
	require.NoError(t, k.ResolveEnabled())

	require.Equal(t, []string{"alpha", "middle", "zeta"}, idsOf(k.Plugins()))
	require.Equal(t, []string{"alpha", "middle", "zeta"}, idsOf(k.EnabledPlugins()))
}

func idsOf(plugins []kernel.Plugin) []string {
	out := make([]string, 0, len(plugins))
	for _, p := range plugins {
		out = append(out, p.ID())
	}
	return out
}

func TestANilPluginIsRefused(t *testing.T) {
	require.ErrorContains(t, kernel.NewKernel().RegisterPlugin(nil), "nil plugin")
}

// Execute is the whole boot: config, plugin config, key validation, resolution,
// command registration, then cobra.
func TestExecuteBootsAndRunsTheCommandItWasGiven(t *testing.T) {
	resetViper()
	defer resetViper()
	t.Setenv("HOME", t.TempDir())

	k := kernel.NewKernel()
	ran := false
	require.NoError(t, k.RegisterPlugin(&runnablePlugin{ran: &ran}))

	var out bytes.Buffer
	k.RootCmd.SetOut(&out)
	k.RootCmd.SetErr(&out)

	require.NoError(t, k.Execute([]string{"greet"}))
	require.True(t, ran)
}

func TestExecuteReadsTheConfigFileItWasPointedAt(t *testing.T) {
	resetViper()
	defer resetViper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "elsewhere.yaml")
	require.NoError(t, os.WriteFile(path, []byte("model: from-the-file\n"), 0o600))

	k := kernel.NewKernel()
	ran := false
	require.NoError(t, k.RegisterPlugin(&runnablePlugin{ran: &ran}))

	var out bytes.Buffer
	k.RootCmd.SetOut(&out)
	k.RootCmd.SetErr(&out)

	require.NoError(t, k.Execute([]string{"greet", "--config", path}))
	require.True(t, ran)
}

// A key no plugin declared is reported. Whether it stops the boot is what
// config.validation decides, and "strict" is what a machine wants.
func TestExecuteStopsOnAConfigKeyNobodyDeclared(t *testing.T) {
	resetViper()
	defer resetViper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	path := filepath.Join(home, "wrong.yaml")
	require.NoError(t, os.WriteFile(path,
		[]byte("config:\n  validation: strict\nnosuchplugin:\n  setting: 1\n"), 0o600))

	k := kernel.NewKernel()
	require.NoError(t, k.RegisterPlugin(&testPlugin{id: "one", def: true}))
	k.RootCmd.SetOut(&bytes.Buffer{})
	k.RootCmd.SetErr(&bytes.Buffer{})

	err := k.Execute([]string{"--config", path})

	require.ErrorContains(t, err, "nosuchplugin")
}

type runnablePlugin struct{ ran *bool }

func (p *runnablePlugin) ID() string                 { return "greeter" }
func (p *runnablePlugin) DefaultEnabled() bool       { return true }
func (p *runnablePlugin) DependsOn() []string        { return nil }
func (p *runnablePlugin) ConfigSchema() []string     { return nil }
func (p *runnablePlugin) MCPTools() []kernel.MCPTool { return nil }
func (p *runnablePlugin) Register(_ *kernel.Kernel) ([]*cobra.Command, error) {
	return []*cobra.Command{{
		Use: "greet",
		RunE: func(*cobra.Command, []string) error {
			*p.ran = true
			return nil
		},
	}}, nil
}
