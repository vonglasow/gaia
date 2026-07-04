package configplugin

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gaia/config"
	"gaia/kernel"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ConfigPlugin struct{}

func NewConfigPlugin() *ConfigPlugin { return &ConfigPlugin{} }

func (p *ConfigPlugin) ID() string           { return "config" }
func (p *ConfigPlugin) DefaultEnabled() bool { return true }
func (p *ConfigPlugin) DependsOn() []string  { return nil }
func (p *ConfigPlugin) ConfigSchema() []string {
	return nil
}

func (p *ConfigPlugin) MCPTools() []kernel.MCPTool { return nil }

func (p *ConfigPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Manage configuration",
	}

	const listMaxValueLen = 80

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List configuration keys and values",
		RunE: func(cmd *cobra.Command, args []string) error {
			short, _ := cmd.Flags().GetBool("short")
			keys := config.FlattenKeys(viper.AllSettings())
			sort.Strings(keys)
			var b strings.Builder
			for _, key := range keys {
				val := viper.Get(key)
				valStr := fmt.Sprintf("%v", val)
				if short && len(valStr) > listMaxValueLen {
					valStr = valStr[:listMaxValueLen] + "..."
				}
				b.WriteString(fmt.Sprintf("%s: %s\n", key, valStr))
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Config", strings.TrimRight(b.String(), "\n"))
		},
	}
	listCmd.Flags().BoolP("short", "s", false, "Truncate long values (e.g. role prompts)")

	getCmd := &cobra.Command{
		Use:   "get [key]",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !viper.IsSet(key) {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Config key %q is not set", key))
			}
			if config.IsListKey(key) {
				val := viper.GetStringSlice(key)
				raw, err := json.Marshal(val)
				if err != nil {
					return err
				}
				return shared.PrintBox(cmd.OutOrStdout(), "Config", string(raw))
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Config", fmt.Sprintf("%v", viper.Get(key)))
		},
	}

	setCmd := &cobra.Command{
		Use:   "set [key] [value]",
		Short: "Set a configuration value",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.SetConfigString(args[0], args[1]); err != nil {
				return err
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Config", fmt.Sprintf("Updated %s", args[0]))
		},
	}

	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create the default configuration file if it does not exist",
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := config.InitConfig(); err != nil {
				return fmt.Errorf("failed to initialize config: %w", err)
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Config", fmt.Sprintf("Configuration file ensured at: %s", config.CfgFile))
		},
	}

	pathCmd := &cobra.Command{
		Use:   "path",
		Short: "Show the configuration file path in use",
		RunE: func(cmd *cobra.Command, args []string) error {
			if config.CfgFile == "" {
				if err := config.InitConfig(); err != nil {
					return err
				}
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Config", config.CfgFile)
		},
	}

	trustCmd := &cobra.Command{
		Use:   "trust [path]",
		Short: "Trust local .gaia.yaml overrides for a repository",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			repoRoot, err := config.ResolveRepositoryRootFromPath(target)
			if err != nil {
				return fmt.Errorf("resolve repository root from %q: %w", target, err)
			}
			if err := config.TrustRepository(repoRoot); err != nil {
				return fmt.Errorf("trust repository %q: %w", repoRoot, err)
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Config",
				fmt.Sprintf("Trusted repository: %s\nLocal overrides file (if present): %s", repoRoot, repoRoot+"/.gaia.yaml"))
		},
	}

	trustedCmd := &cobra.Command{
		Use:   "trusted [path]",
		Short: "List trusted repositories or show trust status",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 1 {
				repoRoot, err := config.ResolveRepositoryRootFromPath(args[0])
				if err != nil {
					return fmt.Errorf("resolve repository root from %q: %w", args[0], err)
				}
				trusted, err := config.IsRepositoryTrusted(repoRoot)
				if err != nil {
					return fmt.Errorf("read trust status for %q: %w", repoRoot, err)
				}
				status := "no"
				if trusted {
					status = "yes"
				}
				return shared.PrintBox(cmd.OutOrStdout(), "Config", fmt.Sprintf("Trusted: %s (%s)", status, repoRoot))
			}
			trustedRepos, err := config.ListTrustedRepositories()
			if err != nil {
				return fmt.Errorf("list trusted repositories: %w", err)
			}
			if len(trustedRepos) == 0 {
				return shared.PrintBox(cmd.OutOrStdout(), "Config", "No trusted repositories found")
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Config", strings.Join(trustedRepos, "\n"))
		},
	}

	untrustCmd := &cobra.Command{
		Use:   "untrust [path]",
		Short: "Remove trust for local .gaia.yaml overrides",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			target := "."
			if len(args) == 1 {
				target = args[0]
			}
			repoRoot, err := config.ResolveRepositoryRootFromPath(target)
			if err != nil {
				return fmt.Errorf("resolve repository root from %q: %w", target, err)
			}
			if err := config.UntrustRepository(repoRoot); err != nil {
				return fmt.Errorf("untrust repository %q: %w", repoRoot, err)
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Config", fmt.Sprintf("Removed trust for repository: %s", repoRoot))
		},
	}

	configCmd.AddCommand(listCmd, getCmd, setCmd, createCmd, pathCmd, trustCmd, trustedCmd, untrustCmd)
	return []*cobra.Command{configCmd}, nil
}
