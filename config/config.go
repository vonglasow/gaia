package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
	"gopkg.in/yaml.v3"
)

var CfgFile string

var kernelKeys = map[string]bool{
	"config.validation": true,
	"debug":             true,
	"cache.refresh":     true,
	"provider":          true,
	"host":              true,
	"port":              true,
	"model":             true,
	"timeout_seconds":   true,
	"plugins.enabled":   true,
	"plugins.disabled":  true,
}

var (
	pluginExactKeys  = map[string]map[string]bool{}
	pluginPrefixKeys = map[string][]string{}
)

// RegisterPluginSchema registers config keys for a plugin.
// Keys must be prefixed with "<plugin>." and may end with ".*" to allow any nested keys.
func RegisterPluginSchema(pluginID string, keys []string) error {
	if pluginID == "" {
		return fmt.Errorf("plugin id is required for schema registration")
	}
	if pluginExactKeys[pluginID] == nil {
		pluginExactKeys[pluginID] = map[string]bool{}
	}
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			continue
		}
		requiredPrefix := pluginID + "."
		if !strings.HasPrefix(key, requiredPrefix) {
			return fmt.Errorf("config key %q must be prefixed with %q", key, requiredPrefix)
		}
		if strings.HasSuffix(key, ".*") {
			prefix := strings.TrimSuffix(key, "*")
			pluginPrefixKeys[pluginID] = append(pluginPrefixKeys[pluginID], prefix)
			continue
		}
		pluginExactKeys[pluginID][key] = true
	}
	return nil
}

// IsValidKey checks if a key is valid for configuration.
func IsValidKey(key string) bool {
	if kernelKeys[key] {
		return true
	}
	for _, keys := range pluginExactKeys {
		if keys[key] {
			return true
		}
	}
	for _, prefixes := range pluginPrefixKeys {
		for _, prefix := range prefixes {
			if strings.HasPrefix(key, prefix) {
				return true
			}
		}
	}
	return false
}

// IsListKey returns true if the key holds a list ([]string) value.
func IsListKey(key string) bool {
	switch key {
	case "plugins.enabled", "plugins.disabled",
		"tools.allow", "tools.allow_patterns", "tools.deny", "tools.deny_patterns",
		"investigate.allowlist", "investigate.denylist":
		return true
	default:
		if strings.HasPrefix(key, "roles.keywords.") {
			return true
		}
		return false
	}
}

func setDefaults() {
	viper.SetDefault("config.validation", "warn")
	viper.SetDefault("plugins.enabled", []string{})
	viper.SetDefault("plugins.disabled", []string{})
	viper.SetDefault("cache.enabled", false)
	viper.SetDefault("cache.refresh", false)
	viper.SetDefault("roles.directory", "")
	viper.SetDefault("sanitize.enabled", false)
	viper.SetDefault("sanitize.level", "light")
	viper.SetDefault("sanitize.max_tokens_after", 0)
	viper.SetDefault("sanitize.log_stats", false)
}

func InitConfig() error {
	if CfgFile == "" {
		if env, ok := os.LookupEnv("GAIA_CONFIG"); ok && env != "" {
			CfgFile = env
		} else {
			homeDir, err := os.UserHomeDir()
			if err != nil {
				return fmt.Errorf("failed to determine home directory: %w", err)
			}
			configDir := filepath.Join(homeDir, ".config", "gaia")
			if err := os.MkdirAll(configDir, 0o755); err != nil {
				return fmt.Errorf("failed to create config directory %s: %w", configDir, err)
			}
			yamlPath := filepath.Join(configDir, "config.yaml")
			ymlPath := filepath.Join(configDir, "config.yml")
			if fileExists(yamlPath) {
				CfgFile = yamlPath
			} else if fileExists(ymlPath) {
				CfgFile = ymlPath
			} else {
				CfgFile = yamlPath
			}
		}
	}
	viper.SetConfigFile(CfgFile)
	viper.SetConfigType("yaml")
	viper.AddConfigPath(".")
	setDefaults()
	if err := viper.ReadInConfig(); err != nil {
		var nf viper.ConfigFileNotFoundError
		var pe *os.PathError
		if errors.As(err, &nf) || (errors.As(err, &pe) && (errors.Is(pe, os.ErrNotExist) || errors.Is(pe, fs.ErrNotExist))) {
			if err := viper.SafeWriteConfigAs(CfgFile); err != nil {
				var already viper.ConfigFileAlreadyExistsError
				if errors.As(err, &already) || os.IsExist(err) {
					if err := viper.ReadInConfig(); err != nil {
						return fmt.Errorf("read config after create race: %w", err)
					}
				} else {
					return fmt.Errorf("create default config: %w", err)
				}
			} else {
				if err := viper.ReadInConfig(); err != nil {
					return fmt.Errorf("read created default config: %w", err)
				}
			}
		} else {
			return fmt.Errorf("read config: %w", err)
		}
	}
	if err := loadTrustedLocalConfig(); err != nil {
		return err
	}
	return nil
}

// SetConfigString sets a config key. For list keys (e.g. plugins.enabled, plugins.disabled),
// value must be a JSON array of strings, e.g. `["a","b"]`.
// For scalar keys, value is stored as-is.
func SetConfigString(key, value string) error {
	if !IsValidKey(key) {
		return fmt.Errorf("invalid config key %q", key)
	}
	if IsListKey(key) {
		valueTrimmed := strings.TrimSpace(value)
		if !strings.HasPrefix(valueTrimmed, "[") {
			return fmt.Errorf("list key %q requires a JSON array of strings, e.g. [\"a\",\"b\"]", key)
		}
		var list []string
		if err := json.Unmarshal([]byte(valueTrimmed), &list); err != nil {
			return fmt.Errorf("list key %q: invalid JSON array: %w", key, err)
		}
		if isKernelKey(key) {
			viper.Set(key, list)
		} else {
			pluginID := pluginIDFromKey(key)
			if pluginID == "" {
				return fmt.Errorf("invalid plugin key %q", key)
			}
			viper.Set(key, list)
			return writePluginConfigValue(pluginID, strings.TrimPrefix(key, pluginID+"."), list)
		}
	} else {
		if isKernelKey(key) {
			viper.Set(key, value)
			if err := viper.WriteConfig(); err != nil {
				return fmt.Errorf("failed to write config file %s: %w", CfgFile, err)
			}
			return nil
		}
		pluginID := pluginIDFromKey(key)
		if pluginID == "" {
			return fmt.Errorf("invalid plugin key %q", key)
		}
		viper.Set(key, value)
		return writePluginConfigValue(pluginID, strings.TrimPrefix(key, pluginID+"."), value)
	}
	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", CfgFile, err)
	}
	return nil
}

// ValidationMode returns the current config validation mode.
// Allowed values: "strict", "warn", "off".
func ValidationMode() string {
	mode := strings.ToLower(strings.TrimSpace(viper.GetString("config.validation")))
	switch mode {
	case "strict", "warn", "off":
		return mode
	default:
		return "warn"
	}
}

// FlattenKeys returns all leaf keys from a nested settings map.
func FlattenKeys(settings map[string]any) []string {
	keys := []string{}
	var walk func(prefix string, value any)
	walk = func(prefix string, value any) {
		switch v := value.(type) {
		case map[string]any:
			for k, child := range v {
				next := k
				if prefix != "" {
					next = prefix + "." + k
				}
				walk(next, child)
			}
		default:
			if prefix != "" {
				keys = append(keys, prefix)
			}
		}
	}
	for k, v := range settings {
		walk(k, v)
	}
	return keys
}

// KeysFromFile loads and flattens keys from a YAML config file.
func KeysFromFile(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil, nil
	}
	var settings map[string]any
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return nil, err
	}
	return FlattenKeys(settings), nil
}

// PluginConfigKeys loads and flattens keys from a plugin config file, returning namespaced keys.
func PluginConfigKeys(pluginID string) ([]string, error) {
	path := pluginConfigPath(pluginID)
	keys, err := KeysFromFile(path)
	if err != nil {
		return nil, err
	}
	if len(keys) == 0 {
		return nil, nil
	}
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		if strings.HasPrefix(key, pluginID+".") {
			out = append(out, key)
			continue
		}
		out = append(out, pluginID+"."+key)
	}
	return out, nil
}

// LoadPluginConfig merges a plugin config file into the current Viper instance.
func LoadPluginConfig(pluginID string) error {
	path := pluginConfigPath(pluginID)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	if strings.TrimSpace(string(data)) == "" {
		return nil
	}
	var settings map[string]any
	if err := yaml.Unmarshal(data, &settings); err != nil {
		return err
	}
	if len(settings) == 0 {
		return nil
	}
	if _, ok := settings[pluginID]; ok {
		return viper.MergeConfigMap(settings)
	}
	return viper.MergeConfigMap(map[string]any{pluginID: settings})
}

func pluginConfigPath(pluginID string) string {
	base := ConfigDir()
	return filepath.Join(base, "plugins", pluginID+".yaml")
}

func ConfigDir() string {
	if CfgFile != "" {
		return filepath.Dir(CfgFile)
	}
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return "."
	}
	return filepath.Join(homeDir, ".config", "gaia")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func isKernelKey(key string) bool {
	return kernelKeys[key]
}

func pluginIDFromKey(key string) string {
	parts := strings.SplitN(key, ".", 2)
	if len(parts) != 2 {
		return ""
	}
	return parts[0]
}

func writePluginConfigValue(pluginID, key string, value any) error {
	dir := filepath.Join(ConfigDir(), "plugins")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	path := pluginConfigPath(pluginID)

	settings := map[string]any{}
	if data, err := os.ReadFile(path); err == nil {
		_ = yaml.Unmarshal(data, &settings)
	}

	segments := strings.Split(key, ".")
	setNestedValue(settings, segments, value)

	out, err := yaml.Marshal(settings)
	if err != nil {
		return err
	}
	return os.WriteFile(path, out, 0o644)
}

func setNestedValue(target map[string]any, path []string, value any) {
	if len(path) == 0 {
		return
	}
	if len(path) == 1 {
		target[path[0]] = value
		return
	}
	next, ok := target[path[0]].(map[string]any)
	if !ok {
		next = map[string]any{}
		target[path[0]] = next
	}
	setNestedValue(next, path[1:], value)
}
