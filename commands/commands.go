package commands

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"gaia/api"
	"gaia/config"
	"log"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	version              string = "dev"
	commitSHA            string = "none"
	buildDate            string = "unknown"
	apiClient            *api.APIClient
	defaultSystemMessage string
)

// InitAPIClient initializes the API client and default system message.
// It also checks and pulls the model if necessary.
func InitAPIClient() error {
	host := viper.GetString("host")
	port := viper.GetInt("port")
	model := viper.GetString("model")
	// Ensure port is converted to string for NewAPIClient
	apiClient = api.NewAPIClient(host, fmt.Sprintf("%d", port), model)

	defaultRoleTemplate := viper.GetString("roles.default")
	if defaultRoleTemplate == "" {
		return fmt.Errorf("default role template ('roles.default') not found in configuration")
	}
	defaultSystemMessage = fmt.Sprintf(defaultRoleTemplate, runtime.GOOS, filepath.Base(os.Getenv("SHELL")))

	fmt.Println("Checking and pulling model if necessary...")
	if err := apiClient.CheckAndPullModel(); err != nil {
		return fmt.Errorf("failed to initialize API client and ensure model exists: %w", err)
	}
	fmt.Println("Model check complete.")
	return nil
}

var rootCmd = &cobra.Command{Use: "gaia"}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Set configuration options",
}

var listCmd = &cobra.Command{
	Use:   "list",
	Short: "List configuration settings",
	Run: func(cmd *cobra.Command, args []string) {
		keys := make([]string, 0, len(viper.AllKeys()))
		keys = append(keys, viper.AllKeys()...)
		sort.Strings(keys)
		for _, key := range keys {
			fmt.Printf("%s: %v\n", key, viper.Get(key))
		}
	},
}

var setCmd = &cobra.Command{
	Use:   "set [key] [value]",
	Short: "Set configuration setting",
	Args:  cobra.ExactArgs(2),
	Run: func(cmd *cobra.Command, args []string) {
		config.SetConfigString(args[0], args[1])
		fmt.Println("Config setting updated", args[0], "to", args[1])
	},
}

var getCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get configuration setting",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(viper.GetString(args[0]))
	},
}

var pathCmd = &cobra.Command{
	Use:   "path",
	Short: "Get configuration path",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(config.CfgFile)
	},
}

var askCmd = &cobra.Command{
	Use:   "ask [string]",
	Short: "Ask to a model",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		msg := ""
		msg += readStdin()
		if len(args) > 0 {
			msg += " " + args[0]
		}
		// Determine the system message
		roleName := viper.GetString("systemrole") // e.g., "code", "shell"
		var finalSystemMessage string
		if roleName != "" {
			roleConfigKey := "roles." + roleName
			roleTemplate := viper.GetString(roleConfigKey)
			if roleTemplate == "" {
				fmt.Fprintf(os.Stderr, "Warning: Role '%s' not found. Using default system message.\n", roleName)
				finalSystemMessage = defaultSystemMessage
			} else {
				// Apply formatting based on which role it is
				if roleName == "shell" || roleName == "default" { // Or any other role that uses the two %s for OS and shell
					finalSystemMessage = fmt.Sprintf(roleTemplate, runtime.GOOS, filepath.Base(os.Getenv("SHELL")))
				} else { // For roles like 'code', 'describe' that don't (or use different templating)
					finalSystemMessage = roleTemplate
				}
			}
		} else {
			finalSystemMessage = defaultSystemMessage
		}

		session := api.NewChatSession()
		if err := apiClient.ProcessMessage(session, msg, finalSystemMessage); err != nil {
			log.Printf("Error processing message: %v\n", err)
		}
	},
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("Gaia %s, commit %s, built at %s\n", version, commitSHA, buildDate)
	},
}

var chatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Start an interactive chat session",
	Run: func(cmd *cobra.Command, args []string) {
		session := api.NewChatSession()
		reader := bufio.NewReader(os.Stdin)
		fmt.Println("Starting chat session. Type 'exit' to end the chat.")
		fmt.Println("----------------------------------------")

		// Use defaultSystemMessage for chat sessions
		// If a specific system message for chat is desired, it can be configured and retrieved here.
		// For now, defaultSystemMessage (which is roles.default formatted) will be used.
		chatSystemMessage := defaultSystemMessage

		for {
			fmt.Print("You: ")
			input, err := reader.ReadString('\n')
			if err != nil {
				if err == io.EOF {
					fmt.Println("\nChat session ended (EOF received).") // This is informational, not an error
					break
				}
				log.Printf("Error reading input: %v\n", err)
				continue
			}

			input = strings.TrimSpace(input)
			if input == "exit" {
				fmt.Println("Chat session ended.")
				break
			}

			if input == "" { // Skip empty input
				continue
			}

			if err := apiClient.ProcessMessage(session, input, chatSystemMessage); err != nil {
				log.Printf("Error processing message: %v\n", err)
			}
			// No need for the extra "----------------------------------------" here,
			// as ProcessMessage now handles printing the AI's response directly.
		}
	},
}

func readStdin() string {
	var stdinLines string
	stat, _ := os.Stdin.Stat()
	if (stat.Mode() & os.ModeCharDevice) == 0 {
		buf := make([]byte, 4096)
		n, _ := os.Stdin.Read(buf)
		stdinLines = string(buf[:n])
	}
	return strings.TrimSpace(stdinLines)
}

func Execute() error {
	configCmd.AddCommand(listCmd, setCmd, getCmd, pathCmd)
	askCmd.Flags().StringP("role", "r", "", "Specify role code (default, describe, code)")
	if err := viper.BindPFlag("systemrole", askCmd.Flags().Lookup("role")); err != nil {
		log.Printf("Error binding flag 'role' to Viper: %v\n", err)
		return err
	}
	rootCmd.AddCommand(configCmd, versionCmd, askCmd, chatCmd)
	return rootCmd.Execute()
}
