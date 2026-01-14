package config_test

import (
	"os"
	"path/filepath"
	"testing"

	"gaia/config"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInitConfig_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	config.CfgFile = filepath.Join(dir, "config.yaml")

	// le fichier n'existe pas encore
	require.NoFileExists(t, config.CfgFile)

	err := config.InitConfig()
	require.NoError(t, err)
	require.FileExists(t, config.CfgFile)

	// Viper doit lire la config
	v := viper.New()
	v.SetConfigFile(config.CfgFile)
	require.NoError(t, v.ReadInConfig())
	assert.Equal(t, "mistral", v.GetString("model"))
}

func TestInitConfig_UsesEnvVar(t *testing.T) {
	dir := t.TempDir()
	envFile := filepath.Join(dir, "config_env.yaml")
	require.NoError(t, os.WriteFile(envFile, []byte{}, 0644))

	old := os.Getenv("GAIA_CONFIG")
	defer func() {
		if err := os.Setenv("GAIA_CONFIG", old); err != nil {
			panic(err)
		}
	}()
	require.NoError(t, os.Setenv("GAIA_CONFIG", envFile))

	config.CfgFile = ""
	err := config.InitConfig()
	require.NoError(t, err)
	assert.Equal(t, envFile, config.CfgFile)
}

func TestSetConfigString_Success(t *testing.T) {
	dir := t.TempDir()
	config.CfgFile = filepath.Join(dir, "config.yaml")

	// écrire YAML minimal pour que la clé existe
	yamlContent := []byte(`
roles:
  default: "test"
systemrole: default
model: "mistral"
`)
	require.NoError(t, os.WriteFile(config.CfgFile, yamlContent, 0644))
	require.NoError(t, config.InitConfig())

	// changer la valeur d'une clé existante
	err := config.SetConfigString("model", "gpt-test")
	require.NoError(t, err)

	v := viper.GetViper()
	assert.Equal(t, "gpt-test", v.GetString("model"))
}

func TestSetConfigString_InvalidKey(t *testing.T) {
	dir := t.TempDir()
	config.CfgFile = filepath.Join(dir, "config.yaml")
	require.NoError(t, config.InitConfig())

	err := config.SetConfigString("invalid_key", "value")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config key")
}
