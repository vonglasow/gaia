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
	AskCmd.Flags().StringP("role", "r", "", "Specify role code (default, describe, code)")
	if err := viper.BindPFlag("systemrole", AskCmd.Flags().Lookup("role")); err != nil {
		return fmt.Errorf("failed to bind role flag: %w", err)
	}
	RootCmd.AddCommand(ConfigCmd, VersionCmd, AskCmd, ChatCmd)
	return RootCmd.Execute()
}

// CallReadStdinForTest allows tests to call the unexported readStdin function
func CallReadStdinForTest() string {
	return readStdin()
}
