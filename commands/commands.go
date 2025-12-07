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
		path := cmd.CommandPath()
		if strings.HasSuffix(path, "config create") {
			return nil
		}
		if err := config.InitConfig(); err != nil {
			return fmt.Errorf("init config: %w", err)
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
		keys := make([]string, 0, len(viper.AllKeys()))
		keys = append(keys, viper.AllKeys()...)
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
	Run: func(cmd *cobra.Command, args []string) {
		config.SetConfigString(args[0], args[1])
		fmt.Println("Config setting updated", args[0], "to", args[1])
	},
}

var GetCmd = &cobra.Command{
	Use:   "get [key]",
	Short: "Get configuration setting",
	Args:  cobra.ExactArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(viper.GetString(args[0]))
	},
}

var PathCmd = &cobra.Command{
	Use:   "path",
	Short: "Get configuration path",
	Args:  cobra.ExactArgs(0),
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println(config.CfgFile)
	},
}

var AskCmd = &cobra.Command{
	Use:   "ask [string]",
	Short: "Ask to a model",
	Args:  cobra.MinimumNArgs(1),
	Run: func(cmd *cobra.Command, args []string) {
		msg := ""
		msg += readStdin()
		if len(args) > 0 {
			msg += " " + args[0]
		}
		if err := api.ProcessMessage(msg); err != nil {
			fmt.Println(err)
		}
	},
}

var VersionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version information",
	Args:  cobra.ExactArgs(0),
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
				fmt.Println("Error processing message:", err)
			}
			fmt.Println("----------------------------------------")
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
		fmt.Printf("Error binding flag to Viper: %v\n", err)
		return err
	}
	RootCmd.AddCommand(ConfigCmd, VersionCmd, AskCmd, ChatCmd)
	return RootCmd.Execute()
}

// CallReadStdinForTest allows tests to call the unexported readStdin function
func CallReadStdinForTest() string {
	return readStdin()
}
