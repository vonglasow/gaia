package config

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/spf13/viper"
)

var CfgFile string
var knownKeys map[string]bool

func applyDefaults() {
	viper.SetDefault("model", "mistral")
	viper.SetDefault("host", "localhost")
	viper.SetDefault("port", 11434)

	// roles.default: Default system message.
	// Expects two %s placeholders: 1. Operating System (e.g., "linux"), 2. Shell name (e.g., "bash").
	viper.SetDefault("roles.default", "You are programming and system administration assistant. You are managing %s operating system with %s shell. Provide short responses in about 100 words, unless you are specifically asked for more details. If you need to store any data, assume it will be stored in the conversation. APPLY MARKDOWN formatting when possible.")

	// roles.describe: Provides a description of a shell command. No placeholders expected.
	viper.SetDefault("roles.describe", "Provide a terse, single sentence description of the given shell command. Describe each argument and option of the command. Provide short responses in about 80 words. APPLY MARKDOWN formatting when possible.")

	// roles.shell: Provides shell commands.
	// Expects two %s placeholders: 1. Operating System (e.g., "linux"), 2. Shell name (e.g., "bash").
	// Example when formatted: "Provide only linux commands for bash without any description."
	viper.SetDefault("roles.shell", "Provide only %s commands for %s without any description. If there is a lack of details, provide the most logical solution. Ensure the output is a valid shell command. If multiple steps are required, try to combine them using &&. Provide only plain text without Markdown formatting. Do not use markdown formatting such as ```.")

	// roles.code: Provides only code output. No placeholders expected.
	viper.SetDefault("roles.code", "Provide only code as output without any description. Provide only code in plain text format without Markdown formatting. Do not include symbols such as ``` or ```python. If there is a lack of details, provide most logical solution. You are not allowed to ask for more details. For example if the prompt is \"Hello world Python\", you should return \"print('Hello world')\".")

	// Populate knownKeys after setting all defaults
	knownKeys = make(map[string]bool)
	for _, key := range viper.AllKeys() { // AllKeys here will get all default keys
		knownKeys[key] = true
	}
}

func InitConfig() error {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("error getting home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, ".config", "gaia")
	if err = os.MkdirAll(configDir, 0755); err != nil {
		return fmt.Errorf("error creating config directory: %w", err)
	}

	CfgFile = filepath.Join(configDir, "config.yaml")

	viper.SetConfigFile(CfgFile)
	viper.SetConfigType("yaml")
	// viper.AddConfigPath(".") // Removed for clarity, CfgFile is a full path

	applyDefaults() // Apply defaults and populate knownKeys

	if err := viper.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); ok {
			log.Printf("Configuration file not found. Creating with default settings at: %s", CfgFile)
			if errWrite := viper.WriteConfigAs(CfgFile); errWrite != nil {
				return fmt.Errorf("failed to write new config file %s: %w", CfgFile, errWrite)
			}
			// configWasCreated = true // This variable is not used anymore
		} else {
			// Some other error occurred reading the config file
			return fmt.Errorf("failed to read config file %s: %w", CfgFile, err)
		}
	}
	// The logic for `if !configWasCreated && err != nil ...` was removed as it's covered by the error handling above.
	// If ReadInConfig fails with anything other than ConfigFileNotFoundError, InitConfig now returns an error.
	// If it was ConfigFileNotFoundError and WriteConfigAs failed, that's also an error return.
	// If both succeed (or ReadInConfig succeeds), we proceed without the old informational message.

	return nil
}

func SetConfigString(key, value string) {
	if !knownKeys[key] {
		fmt.Printf("Warning: Setting configuration for key '%s' which does not have a default value.\n", key)
	}
	viper.Set(key, value)
	if err := viper.WriteConfigAs(CfgFile); err != nil {
		log.Printf("Error writing config file: %v\n", err)
	}
}
