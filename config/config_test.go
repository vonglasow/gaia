package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"gaia/config"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

func resetViper() {
	viper.Reset()
	config.CfgFile = ""
}

func TestInitConfig_CreatesFile(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, config.InitConfig())
	if _, err := os.Stat(config.CfgFile); err != nil {
		t.Fatalf("expected config file, got %v", err)
	}
}

func TestInitConfig_UsesEnvVar(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	cfgPath := filepath.Join(tmpDir, "config.yaml")
	t.Setenv("GAIA_CONFIG", cfgPath)
	require.NoError(t, config.InitConfig())
	if config.CfgFile != cfgPath {
		t.Fatalf("expected config path %q, got %q", cfgPath, config.CfgFile)
	}
}

func TestSetConfigString_ValidPluginKey(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, config.InitConfig())

	require.NoError(t, config.RegisterPluginSchema("ask", []string{"ask.host"}))
	require.NoError(t, config.SetConfigString("ask.host", "localhost"))
	require.Equal(t, "localhost", viper.GetString("ask.host"))
}

func TestSetConfigString_InvalidKey(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, config.InitConfig())
	err := config.SetConfigString("invalid.key", "value")
	require.Error(t, err)
}

func TestSetConfigString_ListKey(t *testing.T) {
	resetViper()
	defer resetViper()

	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	require.NoError(t, config.InitConfig())
	require.NoError(t, config.SetConfigString("plugins.enabled", `["ask","chat"]`))
	require.Equal(t, []string{"ask", "chat"}, viper.GetStringSlice("plugins.enabled"))
}

func TestRegisterPluginSchema_RequiresPrefix(t *testing.T) {
	resetViper()
	defer resetViper()

	err := config.RegisterPluginSchema("ask", []string{"other.key"})
	require.Error(t, err)
}

// A list in YAML arrives as []interface{}; set from Go it is []string. Both are
// the same list, and every caller reading an allowlist depends on that.
func TestAListIsReadInBothShapesViperCanReturn(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("agent.denylist", []string{"sudo", "rm"})
	require.Equal(t, []string{"sudo", "rm"}, config.StringList("agent.denylist"))

	viper.Set("agent.allowlist", []interface{}{"git", "ls"})
	require.Equal(t, []string{"git", "ls"}, config.StringList("agent.allowlist"),
		"this is the shape a YAML file actually produces")
}

func TestAListDropsWhatIsNotAString(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("agent.denylist", []interface{}{"sudo", 42, nil, "rm"})

	require.Equal(t, []string{"sudo", "rm"}, config.StringList("agent.denylist"))
}

func TestASingleStringIsNotAList(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("agent.denylist", "sudo")

	require.Nil(t, config.StringList("agent.denylist"),
		"guessing that it is one would silently narrow the list to one entry")
}

func TestAKeyNobodySetIsNotAList(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.Nil(t, config.StringList("agent.denylist"))
}
