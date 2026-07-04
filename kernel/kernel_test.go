package kernel_test

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"gaia/config"
	"gaia/kernel"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

type testPlugin struct {
	id      string
	def     bool
	deps    []string
	schema  []string
	cmdName string
}

func (p *testPlugin) ID() string                { return p.id }
func (p *testPlugin) DefaultEnabled() bool      { return p.def }
func (p *testPlugin) DependsOn() []string       { return p.deps }
func (p *testPlugin) ConfigSchema() []string    { return p.schema }
func (p *testPlugin) MCPTools() []kernel.MCPTool { return nil }
func (p *testPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	if p.cmdName == "" {
		return nil, nil
	}
	cmd := &cobra.Command{Use: p.cmdName}
	return []*cobra.Command{cmd}, nil
}

func resetViper() {
	viper.Reset()
	config.CfgFile = ""
}

func TestResolveEnabled_DefaultsAndOverrides(t *testing.T) {
	resetViper()
	defer resetViper()

	k := kernel.NewKernel()
	require.NoError(t, k.RegisterPlugin(&testPlugin{id: "ask", def: true}))
	require.NoError(t, k.RegisterPlugin(&testPlugin{id: "chat", def: false}))

	viper.Set("plugins.enabled", []string{"chat"})
	viper.Set("plugins.disabled", []string{"ask"})

	require.NoError(t, k.ResolveEnabled())
	require.False(t, k.IsEnabled("ask"))
	require.True(t, k.IsEnabled("chat"))
}

func TestResolveEnabled_DependencyEnforced(t *testing.T) {
	resetViper()
	defer resetViper()

	k := kernel.NewKernel()
	require.NoError(t, k.RegisterPlugin(&testPlugin{id: "core", def: false}))
	require.NoError(t, k.RegisterPlugin(&testPlugin{id: "addon", def: true, deps: []string{"core"}}))

	err := k.ResolveEnabled()
	require.Error(t, err)
	require.Contains(t, err.Error(), "requires")
}

func TestValidateConfigKeys(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, os.WriteFile(cfgPath, []byte("ask:\n  prompt: ok\n  extra: nope\n"), 0o644))
	config.CfgFile = cfgPath

	k := kernel.NewKernel()
	require.NoError(t, k.RegisterPlugin(&testPlugin{id: "ask", def: true, schema: []string{"ask.prompt"}}))
	require.NoError(t, config.InitConfig())
	viper.Set("config.validation", "strict")

	err := k.ValidateConfigKeys()
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid config key")
}

func TestRegisterEnabledCommands_HelpShowsEnabled(t *testing.T) {
	resetViper()
	defer resetViper()

	k := kernel.NewKernel()
	require.NoError(t, k.RegisterPlugin(&testPlugin{id: "ask", def: true, cmdName: "ask"}))
	require.NoError(t, k.RegisterPlugin(&testPlugin{id: "chat", def: false, cmdName: "chat"}))

	require.NoError(t, k.ResolveEnabled())
	require.NoError(t, k.RegisterEnabledCommands())

	buf := &bytes.Buffer{}
	k.RootCmd.SetOut(buf)
	k.RootCmd.SetArgs([]string{"--help"})
	require.NoError(t, k.RootCmd.Execute())

	out := buf.String()
	require.Contains(t, out, "ask")
	require.NotContains(t, out, "chat")
}
