package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

var CfgFile string

var validKeys = map[string]bool{
	"model": true, "host": true, "port": true,
	"roles.default": true, "roles.describe": true, "roles.shell": true, "roles.code": true,
	"roles.commit": true, "roles.branch": true,
	"cache.enabled": true, "cache.dir": true,
	"auto_role.enabled": true, "auto_role.mode": true,
}

// IsValidKey checks if a key is valid for configuration
// This allows dynamic keys like auto_role.keywords.* and roles.*
func IsValidKey(key string) bool {
	// Check exact matches first
	if validKeys[key] {
		return true
	}

	// Allow any roles.* key
	if strings.HasPrefix(key, "roles.") {
		return true
	}

	// Allow any auto_role.keywords.* key
	if strings.HasPrefix(key, "auto_role.keywords.") {
		return true
	}

	return false
}

func setDefaults() {
	viper.SetDefault("model", "mistral")
	viper.SetDefault("host", "localhost")
	viper.SetDefault("port", 11434)
	viper.SetDefault("cache.enabled", true)
	viper.SetDefault("cache.bypass", false)
	viper.SetDefault("cache.refresh", false)
	if homeDir, err := os.UserHomeDir(); err == nil {
		viper.SetDefault("cache.dir", filepath.Join(homeDir, ".config", "gaia", "cache"))
	} else {
		viper.SetDefault("cache.dir", ".gaia-cache")
	}
	viper.SetDefault("roles.default", "You are programming and system administration assistant. You are managing %s operating system with %s shell. Provide short responses in about 100 words, unless you are specifically asked for more details. If you need to store any data, assume it will be stored in the conversation. APPLY MARKDOWN formatting when possible.")
	viper.SetDefault("roles.describe", "Provide a terse, single sentence description of the given shell command. Describe each argument and option of the command. Provide short responses in about 80 words. APPLY MARKDOWN formatting when possible.")
	viper.SetDefault("roles.shell", "Provide only %s commands for %s without any description. If there is a lack of details, provide the most logical solution. Ensure the output is a valid shell command. If multiple steps are required, try to combine them using &&. Provide only plain text without Markdown formatting. Do not use markdown formatting such as ```.")
	viper.SetDefault("roles.code", "Provide only code as output without any description. Provide only code in plain text format without Markdown formatting. Do not include symbols such as ``` or ```python. If there is a lack of details, provide most logical solution. You are not allowed to ask for more details. For example if the prompt is \"Hello world Python\", you should return \"print('Hello world')\".")
	viper.SetDefault("roles.commit", "Generate a conventional commit message based on the provided git diff. The message must have multiple lines: first line is the title (type: subject format), followed by a blank line, then a detailed description on multiple lines. Title format: start with a type (feat, fix, docs, style, refactor, test, chore), followed by a colon and space, then a brief description in lowercase. The description should explain what and why, not how. Do not include markdown formatting, code blocks, or explanations. Only return the commit message itself.")
	viper.SetDefault("roles.branch", "Generate a concise branch name based on the provided git diff or description. The branch name should be lowercase, use hyphens to separate words, and be descriptive but short (max 50 characters). Follow common patterns like: feature/description, fix/description, refactor/description. Do not include markdown formatting, code blocks, or explanations. Only return the branch name itself.")
	// Default tool configurations
	viper.SetDefault("tools.git.commit.context_command", "git diff --staged")
	viper.SetDefault("tools.git.commit.role", "commit")
	viper.SetDefault("tools.git.commit.execute_command", "git commit -F {file}")
	viper.SetDefault("tools.git.branch.context_command", "git diff")
	viper.SetDefault("tools.git.branch.role", "branch")
	viper.SetDefault("tools.git.branch.execute_command", "git checkout -b {response}")
	// Auto-role detection defaults
	viper.SetDefault("auto_role.enabled", true)
	viper.SetDefault("auto_role.mode", "hybrid") // off | heuristic | hybrid

	// Default keywords for role detection
	viper.SetDefault("auto_role.keywords.shell", []string{
		"command", "run", "execute", "terminal", "bash", "zsh", "sh", "shell",
		"cd", "ls", "grep", "find", "mkdir", "rm", "cp", "mv", "cat", "echo",
		"sudo", "chmod", "chown", "ps", "kill", "pkill", "systemctl", "service",
		"install", "uninstall", "package", "apt", "yum", "brew", "pip", "npm",
	})
	viper.SetDefault("auto_role.keywords.code", []string{
		"function", "class", "def", "import", "return", "if", "else", "for", "while",
		"variable", "array", "list", "dict", "string", "int", "bool", "type",
		"python", "javascript", "java", "go", "rust", "c++", "c#", "php", "ruby",
		"code", "programming", "algorithm", "api", "endpoint", "json", "xml",
		"database", "sql", "query", "table", "schema", "migration",
	})
	viper.SetDefault("auto_role.keywords.describe", []string{
		"what", "what does", "explain", "describe", "meaning", "definition",
		"how does", "tell me about", "what is", "what are", "help me understand",
	})
	viper.SetDefault("auto_role.keywords.commit", []string{
		"commit message", "generate commit", "create commit", "write commit", "make commit",
		"conventional commit", "changelog", "commit msg", "git commit message",
		"commit", // Keep as fallback but lower priority
	})
	viper.SetDefault("auto_role.keywords.branch", []string{
		"create branch", "new branch", "make branch", "generate branch", "branch name",
		"git branch", "checkout branch", "switch branch",
		"branch", // Keep as fallback but lower priority
	})
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
	if !IsValidKey(key) {
		return fmt.Errorf("invalid config key '%s'. Valid keys include: model, host, port, cache.enabled, cache.dir, roles.*, auto_role.enabled, auto_role.mode, auto_role.keywords.*", key)
	}
	viper.Set(key, value)
	if err := viper.WriteConfig(); err != nil {
		return fmt.Errorf("failed to write config file %s: %w", CfgFile, err)
	}
	return nil
}
