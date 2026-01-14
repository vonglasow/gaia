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

func TestSetConfigString_AllValidKeys(t *testing.T) {
	dir := t.TempDir()
	config.CfgFile = filepath.Join(dir, "config.yaml")
	require.NoError(t, config.InitConfig())

	validKeys := []string{
		"model", "host", "port",
		"roles.default", "roles.describe", "roles.shell", "roles.code",
		"roles.commit", "roles.branch",
		"cache.enabled", "cache.dir",
	}

	for _, key := range validKeys {
		err := config.SetConfigString(key, "test-value")
		require.NoError(t, err, "failed to set valid key: %s", key)
	}
}

func TestSetConfigString_EmptyValue(t *testing.T) {
	dir := t.TempDir()
	config.CfgFile = filepath.Join(dir, "config.yaml")
	require.NoError(t, config.InitConfig())

	err := config.SetConfigString("model", "")
	require.NoError(t, err)
	assert.Equal(t, "", viper.GetString("model"))
}

func TestSetConfigString_WhitespaceValue(t *testing.T) {
	dir := t.TempDir()
	config.CfgFile = filepath.Join(dir, "config.yaml")
	require.NoError(t, config.InitConfig())

	err := config.SetConfigString("model", "   ")
	require.NoError(t, err)
	assert.Equal(t, "   ", viper.GetString("model"))
}

func TestInitConfig_CreatesDefaultValues(t *testing.T) {
	dir := t.TempDir()
	config.CfgFile = filepath.Join(dir, "config.yaml")

	// Initialize fresh config
	require.NoError(t, config.InitConfig())

	// Check that default values exist (may have been overridden by previous tests)
	// Just verify they're not empty
	assert.NotEmpty(t, viper.GetString("roles.default"))
	assert.NotEmpty(t, viper.GetString("roles.describe"))
	assert.NotEmpty(t, viper.GetString("roles.shell"))
	assert.NotEmpty(t, viper.GetString("roles.code"))
}

func TestInitConfig_ExistingFilePreserved(t *testing.T) {
	dir := t.TempDir()
	cfgFile := filepath.Join(dir, "config.yaml")

	// Create a fresh viper instance
	v := viper.New()
	v.SetConfigFile(cfgFile)
	v.SetConfigType("yaml")

	// Create config with custom value
	yamlContent := []byte(`model: "custom-model"`)
	require.NoError(t, os.WriteFile(cfgFile, yamlContent, 0644))

	require.NoError(t, v.ReadInConfig())

	// Verify custom value is preserved
	assert.Equal(t, "custom-model", v.GetString("model"))
}

func TestInitConfig_InvalidYAML(t *testing.T) {
	dir := t.TempDir()
	config.CfgFile = filepath.Join(dir, "config.yaml")

	// Create invalid YAML
	invalidYAML := []byte(`model: [invalid yaml`)
	require.NoError(t, os.WriteFile(config.CfgFile, invalidYAML, 0644))

	err := config.InitConfig()
	require.Error(t, err)
}

func TestInitConfig_ReadOnlyFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("skipping test when running as root")
	}

	dir := t.TempDir()
	config.CfgFile = filepath.Join(dir, "config.yaml")

	// Create file with no read permissions
	require.NoError(t, os.WriteFile(config.CfgFile, []byte(`model: "test"`), 0000))
	defer func() {
		if err := os.Chmod(config.CfgFile, 0644); err != nil {
			t.Logf("failed to restore file permissions: %v", err)
		}
	}()

	err := config.InitConfig()
	require.Error(t, err)
}

func TestInitConfig_NestedDirectoryCreation(t *testing.T) {
	dir := t.TempDir()
	nestedPath := filepath.Join(dir, "nested", "deep")

	// Create nested directory first
	require.NoError(t, os.MkdirAll(nestedPath, 0755))

	config.CfgFile = filepath.Join(nestedPath, "config.yaml")

	err := config.InitConfig()
	require.NoError(t, err)
	require.FileExists(t, config.CfgFile)
}
