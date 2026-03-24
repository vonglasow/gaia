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
