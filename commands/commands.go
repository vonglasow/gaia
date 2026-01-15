package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"gaia/api"
	"gaia/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version   string = "dev"
	commitSHA string = "none"
	buildDate string = "unknown"
)

var RootCmd = &cobra.Command{
	Use:   "gaia",
	Short: "Gaia CLI",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		if !strings.HasSuffix(cmd.CommandPath(), "config create") {
			if err := config.InitConfig(); err != nil {
				return fmt.Errorf("init config: %w", err)
			}
		}
		return nil
	},
}

var ConfigCmd = &cobra.Command{
	Use:   "config",
	Short: "Set configuration options",
}

var CacheCmd = &cobra.Command{
	Use:   "cache",
	Short: "Manage local response cache",
}

var CacheClearCmd = &cobra.Command{
	Use:   "clear",
	Short: "Clear local response cache",
	RunE: func(cmd *cobra.Command, args []string) error {
		removed, err := api.ClearCache()
		if err != nil {
			return err
		}
		fmt.Printf("Removed %d cache entries\n", removed)
		return nil
	},
}

var CacheStatsCmd = &cobra.Command{
	Use:   "stats",
	Short: "Show cache statistics",
	RunE: func(cmd *cobra.Command, args []string) error {
		stats, err := api.CacheStats()
		if err != nil {
			return err
		}
		fmt.Printf("Entries: %d\nSize: %d bytes\n", stats.Count, stats.SizeBytes)
		return nil
	},
}

var CacheListCmd = &cobra.Command{
	Use:   "list",
	Short: "List cache entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := api.ListCacheEntries()
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("No cache entries found")
			return nil
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Key < entries[j].Key
		})
		for _, entry := range entries {
			fmt.Printf("%s\t%s\t%d bytes\n", entry.Key, entry.CreatedAt.Format(time.RFC3339), entry.SizeBytes)
		}
		return nil
	},
}

var CacheDumpCmd = &cobra.Command{
	Use:   "dump",
	Short: "Print all cache entries",
	RunE: func(cmd *cobra.Command, args []string) error {
		entries, err := api.ReadCacheEntries()
		if err != nil {
			return err
		}
		if len(entries) == 0 {
			fmt.Println("No cache entries found")
			return nil
		}
		sort.Slice(entries, func(i, j int) bool {
			return entries[i].Key < entries[j].Key
		})
		for _, entry := range entries {
			fmt.Printf("Key: %s\nCreatedAt: %s\nResponse:\n%s\n\n", entry.Key, entry.CreatedAt.Format(time.RFC3339), entry.Response)
		}
		return nil
	},
}

var ListCmd = &cobra.Command{
	Use:   "list",
	Short: "List configuration settings",
	Run: func(cmd *cobra.Command, args []string) {
		keys := viper.AllKeys()
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Printf("%s: %v\n", key, viper.Get(key))
		}
	},
}

var CreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create the default configuration file if it does not exist",
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.InitConfig(); err != nil {
			return fmt.Errorf("failed to initialize config: %w", err)
		}
		fmt.Printf("Configuration file ensured at: %s\n", config.CfgFile)
		return nil
	},
}

var SetCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set configuration setting",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		if err := config.SetConfigString(args[0], args[1]); err != nil {
			return err
		}
		fmt.Println("Config setting updated", args[0], "to", args[1])
		return nil
	},
}

var GetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get configuration setting",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		key := args[0]
		if !viper.IsSet(key) {
			return fmt.Errorf("configuration key '%s' is not set. Use 'gaia config list' to see available keys", key)
		}
		fmt.Println(viper.GetString(key))
		return nil
	},
}

var PathCmd = &cobra.Command{
	Use:   "path",
	Short: "Get configuration path",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(config.CfgFile)
	},
}

var AskCmd = &cobra.Command{
	Use:   "ask [string]",
	Short: "Ask to a model",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		msg := readStdin()
		if len(args) > 0 {
			if msg != "" {
				msg += " "
			}
			msg += args[0]
		}
		if strings.TrimSpace(msg) == "" {
			fmt.Fprintf(os.Stderr, "Error: no message provided. Please provide a message as an argument or via stdin.\n")
			return
		}
		if err := api.ProcessMessage(msg); err != nil {
			fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		}
	},
}

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Gaia %s, commit %s, built at %s\n", version, commitSHA, buildDate)
	},
}

var ChatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session",
	Run: func(cmd *cobra.Command, args []string) {
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("Starting chat session. Type 'exit' to end the chat.")
		fmt.Println("----------------------------------------")

		for {
			fmt.Print("You: ")
			input, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					fmt.Println("\nChat session ended (EOF received).")
					break
				}
				fmt.Println("Error reading input:", err)
				continue
			}

			input = strings.TrimSpace(input)
			if input == "exit" {
				fmt.Println("Chat session ended.")
				break
			}

			if err := api.ProcessMessage(input); err != nil {
				fmt.Fprintf(os.Stderr, "Error processing message: %v\n", err)
				fmt.Println("You can continue chatting or type 'exit' to end the session.")
			}
			fmt.Println("----------------------------------------")
		}
	},
}

func readStdin() string {
	stat, err := os.Stdin.Stat()
	if err != nil {
		// If we can't stat stdin, assume it's not available and return empty
		return ""
	}
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		buf := make([]byte, 4096)
		n, err := os.Stdin.Read(buf)
		if err != nil && err != io.EOF {
			// Log but don't fail - return what we got
			fmt.Fprintf(os.Stderr, "Warning: error reading from stdin: %v\n", err)
		}
		if n > 0 {
			return strings.TrimSpace(string(buf[:n]))
		}
	}
	return ""
}

func init() {
	RootCmd.PersistentFlags().StringVarP(
		&config.CfgFile,
		"config",
		"c",
		"",
		"Path to an alternative YAML configuration file (or $GAIA_CONFIG)",
	)
}

func Execute() error {
	ConfigCmd.AddCommand(ListCmd, SetCmd, GetCmd, PathCmd, CreateCmd)
	CacheCmd.AddCommand(CacheClearCmd, CacheStatsCmd, CacheListCmd, CacheDumpCmd)
	AskCmd.Flags().StringP("role", "r", "", "Specify role code (default, describe, code)")
	if err := viper.BindPFlag("systemrole", AskCmd.Flags().Lookup("role")); err != nil {
		return fmt.Errorf("failed to bind role flag: %w", err)
	}
	RootCmd.PersistentFlags().Bool("no-cache", false, "Bypass local response cache")
	if err := viper.BindPFlag("cache.bypass", RootCmd.PersistentFlags().Lookup("no-cache")); err != nil {
		return fmt.Errorf("failed to bind no-cache flag: %w", err)
	}
	RootCmd.PersistentFlags().Bool("refresh-cache", false, "Regenerate and overwrite cache entries")
	if err := viper.BindPFlag("cache.refresh", RootCmd.PersistentFlags().Lookup("refresh-cache")); err != nil {
		return fmt.Errorf("failed to bind refresh-cache flag: %w", err)
	}
	RootCmd.AddCommand(ConfigCmd, CacheCmd, VersionCmd, AskCmd, ChatCmd, ToolCmd)
	return RootCmd.Execute()
}

// CallReadStdinForTest allows tests to call the unexported readStdin function
func CallReadStdinForTest() string {
	return readStdin()
}

// resetTerminal resets the terminal state to fix issues after interactive commands like vim
func resetTerminal() {
	// Use stty sane to reset terminal to a known good state
	resetCmd := exec.Command("stty", "sane")
	resetCmd.Stdin = os.Stdin
	resetCmd.Stdout = os.Stdout
	resetCmd.Stderr = os.Stderr
	_ = resetCmd.Run() // Ignore errors - if stty fails, terminal might not be a TTY
}

// executeExternalCommand runs an external command and returns its output
func executeExternalCommand(command string) (string, error) {
	parts := strings.Fields(command)
	if len(parts) == 0 {
		return "", fmt.Errorf("empty command")
	}

	cmd := exec.Command(parts[0], parts[1:]...)
	output, err := cmd.Output()
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			// Some commands return non-zero exit codes for valid reasons (e.g., git diff with no changes)
			if exitErr.ExitCode() == 1 {
				return strings.TrimSpace(string(output)), nil
			}
			return "", fmt.Errorf("command failed with exit code %d: %w", exitErr.ExitCode(), err)
		}
		return "", fmt.Errorf("failed to execute command: %w", err)
	}

	return strings.TrimSpace(string(output)), nil
}

// getToolActionConfig retrieves the configuration for a specific tool action
func getToolActionConfig(tool, action string) (map[string]interface{}, error) {
	key := fmt.Sprintf("tools.%s.%s", tool, action)
	if !viper.IsSet(key) {
		return nil, fmt.Errorf("tool action '%s.%s' is not configured. Use 'gaia config list' to see available tools", tool, action)
	}

	config := viper.GetStringMap(key)
	if len(config) == 0 {
		return nil, fmt.Errorf("tool action '%s.%s' has no configuration", tool, action)
	}

	return config, nil
}

// promptForContext allows user to add or modify context
func promptForContext(initialContext string) (string, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n--- Current Context ---")
	if initialContext == "" {
		fmt.Println("(no context)")
	} else {
		// Show context with line numbers or truncate if too long
		lines := strings.Split(initialContext, "\n")
		if len(lines) > 20 {
			fmt.Println(strings.Join(lines[:20], "\n"))
			fmt.Printf("... (%d more lines)\n", len(lines)-20)
		} else {
			fmt.Println(initialContext)
		}
	}
	fmt.Println("\nOptions:")
	fmt.Println("  [Enter] - Use current context as-is")
	fmt.Println("  [text]  - Replace context with new text")
	fmt.Println("  [+text] - Append text to context")
	fmt.Println("  [q]     - Quit")
	fmt.Print("\n> ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return initialContext, err
	}

	input = strings.TrimSpace(input)
	if input == "q" || input == "quit" {
		return "", fmt.Errorf("cancelled by user")
	}

	if input == "" {
		return initialContext, nil
	}

	// Check if user wants to append
	if strings.HasPrefix(input, "+") {
		appendText := strings.TrimPrefix(input, "+")
		if initialContext != "" {
			return fmt.Sprintf("%s\n\n%s", initialContext, strings.TrimSpace(appendText)), nil
		}
		return strings.TrimSpace(appendText), nil
	}

	// Replace context
	return input, nil
}

// promptForConfirmation asks user to confirm before executing
func promptForConfirmation(message string) (bool, error) {
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n--- Generated Message ---")
	fmt.Println(message)
	fmt.Println("\nOptions:")
	fmt.Println("  [y/Enter] - Confirm and execute")
	fmt.Println("  [n]       - Cancel")
	fmt.Print("\n> ")

	input, err := reader.ReadString('\n')
	if err != nil {
		return false, err
	}

	input = strings.ToLower(strings.TrimSpace(input))
	return input == "y" || input == "yes" || input == "", nil
}

// executeToolAction executes a configured tool action
func executeToolAction(tool, action string, args []string) error {
	config, err := getToolActionConfig(tool, action)
	if err != nil {
		return err
	}

	// Get context command if configured
	var context string
	if contextCmd, ok := config["context_command"].(string); ok && contextCmd != "" {
		context, err = executeExternalCommand(contextCmd)
		if err != nil {
			return fmt.Errorf("failed to get context: %w", err)
		}
	}

	// Allow user to add/modify context
	context, err = promptForContext(context)
	if err != nil {
		return err
	}

	// Build prompt
	var prompt string
	if len(args) > 0 {
		// Use provided arguments as description
		prompt = strings.Join(args, " ")
		if context != "" {
			prompt = fmt.Sprintf("%s\n\nContext:\n%s", prompt, context)
		}
	} else if context != "" {
		// Use context only
		prompt = context
	} else {
		return fmt.Errorf("no context or arguments provided for tool action '%s.%s'", tool, action)
	}

	// Get role from config
	role, ok := config["role"].(string)
	if !ok || role == "" {
		role = "default"
	}

	// Temporarily set the role
	oldRole := viper.GetString("systemrole")
	viper.Set("systemrole", role)
	defer viper.Set("systemrole", oldRole)

	// Process message with AI
	oldChatHistory := api.GetChatHistory()
	api.ClearChatHistory()
	defer func() {
		api.SetChatHistory(oldChatHistory)
	}()

	response, err := api.ProcessMessageWithResponse(prompt)
	if err != nil {
		return fmt.Errorf("failed to generate response: %w", err)
	}

	// Ask for confirmation before executing
	confirmed, err := promptForConfirmation(response)
	if err != nil {
		return err
	}
	if !confirmed {
		fmt.Println("Cancelled.")
		return nil
	}

	// Execute command if configured
	if executeCmd, ok := config["execute_command"].(string); ok && executeCmd != "" {
		// Check if command uses {file} placeholder (for multi-line content)
		if strings.Contains(executeCmd, "{file}") {
			// Create temporary file with the response
			tmpFile, err := os.CreateTemp("", "gaia-*.txt")
			if err != nil {
				return fmt.Errorf("failed to create temporary file: %w", err)
			}
			tmpFileName := tmpFile.Name()
			defer func() {
				if err := os.Remove(tmpFileName); err != nil {
					fmt.Fprintf(os.Stderr, "Warning: failed to remove temporary file %s: %v\n", tmpFileName, err)
				}
			}()

			if _, err := tmpFile.WriteString(response); err != nil {
				_ = tmpFile.Close()
				return fmt.Errorf("failed to write to temporary file: %w", err)
			}
			if err := tmpFile.Close(); err != nil {
				return fmt.Errorf("failed to close temporary file: %w", err)
			}

			// Replace {file} with the temporary file path
			finalCmd := strings.ReplaceAll(executeCmd, "{file}", tmpFile.Name())
			finalCmd = strings.ReplaceAll(finalCmd, "{response}", tmpFile.Name())
			finalCmd = strings.ReplaceAll(finalCmd, "{output}", tmpFile.Name())

			// Execute the command
			parts := strings.Fields(finalCmd)
			if len(parts) == 0 {
				return fmt.Errorf("invalid execute_command in configuration")
			}

			cmd := exec.Command(parts[0], parts[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin // Required for interactive commands like vim
			err = cmd.Run()
			// Reset terminal state after interactive command (e.g., vim)
			resetTerminal()
			if err != nil {
				return fmt.Errorf("failed to execute command '%s': %w", finalCmd, err)
			}
		} else {
			// Replace {response} placeholder with the AI response (for single-line commands)
			finalCmd := strings.ReplaceAll(executeCmd, "{response}", strings.TrimSpace(response))
			finalCmd = strings.ReplaceAll(finalCmd, "{output}", strings.TrimSpace(response))

			// Execute the command
			parts := strings.Fields(finalCmd)
			if len(parts) == 0 {
				return fmt.Errorf("invalid execute_command in configuration")
			}

			cmd := exec.Command(parts[0], parts[1:]...)
			cmd.Stdout = os.Stdout
			cmd.Stderr = os.Stderr
			cmd.Stdin = os.Stdin // Required for interactive commands
			err = cmd.Run()
			// Reset terminal state after command execution
			resetTerminal()
			if err != nil {
				return fmt.Errorf("failed to execute command '%s': %w", finalCmd, err)
			}
		}
	} else {
		// Just print the response
		fmt.Println(response)
	}

	return nil
}

var ToolCmd = &cobra.Command{
	Use:   "tool <tool> <action> [args...]",
	Short: "Execute a configured external tool action",
	Long: `Execute a configured external tool action. Tools and actions are defined in the configuration file.

Example:
  gaia tool git commit
  gaia tool git branch "add user authentication"

Configuration format in config.yaml:
  tools:
    git:
      commit:
        context_command: "git diff --staged"
        role: "commit"
        execute_command: "git commit -F {file}"
      branch:
        context_command: "git diff"
        role: "branch"
        execute_command: "git checkout -b {response}"

Note: Use {file} placeholder for multi-line content (like commit messages).
Use {response} for single-line content (like branch names).`,
	Args: cobra.MinimumNArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		tool := args[0]
		action := args[1]
		actionArgs := args[2:]

		return executeToolAction(tool, action, actionArgs)
	},
}
