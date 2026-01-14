package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

var CfgFile string

var validKeys = map[string]bool{
	"model": true, "host": true, "port": true,
	"roles.default": true, "roles.describe": true, "roles.shell": true, "roles.code": true,
}

func setDefaults() {
	viper.SetDefault("model", "mistral")
	viper.SetDefault("host", "localhost")
	viper.SetDefault("port", 11434)
	viper.SetDefault("roles.default", "You are programming and system administration assistant. You are managing %s operating system with %s shell. Provide short responses in about 100 words, unless you are specifically asked for more details. If you need to store any data, assume it will be stored in the conversation. APPLY MARKDOWN formatting when possible.")
	viper.SetDefault("roles.describe", "Provide a terse, single sentence description of the given shell command. Describe each argument and option of the command. Provide short responses in about 80 words. APPLY MARKDOWN formatting when possible.")
	viper.SetDefault("roles.shell", "Provide only %s commands for %s without any description. If there is a lack of details, provide the most logical solution. Ensure the output is a valid shell command. If multiple steps are required, try to combine them using &&. Provide only plain text without Markdown formatting. Do not use markdown formatting such as ```.")
	viper.SetDefault("roles.code", "Provide only code as output without any description. Provide only code in plain text format without Markdown formatting. Do not include symbols such as ``` or ```python. If there is a lack of details, provide most logical solution. You are not allowed to ask for more details. For example if the prompt is \"Hello world Python\", you should return \"print('Hello world')\".")
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
			CfgFile = filepath.Join(configDir, "config.yaml")
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
					return nil
				}
				return fmt.Errorf("create default config: %w", err)
			}
			return nil
		}
		return fmt.Errorf("read config: %w", err)
	}
	return nil
}

func SetConfigString(key, value string) error {
	if !validKeys[key] {
		return fmt.Errorf("invalid config key '%s'. Valid keys are: model, host, port, roles.default, roles.describe, roles.shell, roles.code", key)
	}
	viper.Set(key, value)
	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", CfgFile, err)
	}
	return nil
}
