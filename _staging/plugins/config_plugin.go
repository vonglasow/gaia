package plugins

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"gaia/config"
	"gaia/internal/tui"
	"gaia/kernel"

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

func (p *ConfigPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	configCmd := &cobra.Command{
		Use:   "config",
		Short: "Get and set configuration",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List configuration keys and values",
		RunE: func(cmd *cobra.Command, args []string) error {
			keys := config.FlattenKeys(viper.AllSettings())
			sort.Strings(keys)
			var b strings.Builder
			for _, key := range keys {
				val := viper.Get(key)
				b.WriteString(fmt.Sprintf("%s: %v\n", key, val))
			}
			return tui.PrintBox(cmd.OutOrStdout(), "Config", strings.TrimRight(b.String(), "\n"))
		},
	}

	getCmd := &cobra.Command{
		Use:   "get [key]",
		Short: "Get a configuration value",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			key := args[0]
			if !viper.IsSet(key) {
				return tui.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Config key %q is not set", key))
			}
			if config.IsListKey(key) {
				val := viper.GetStringSlice(key)
				raw, err := json.Marshal(val)
				if err != nil {
					return err
				}
				return tui.PrintBox(cmd.OutOrStdout(), "Config", string(raw))
			}
			return tui.PrintBox(cmd.OutOrStdout(), "Config", fmt.Sprintf("%v", viper.Get(key)))
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
			return tui.PrintBox(cmd.OutOrStdout(), "Config", fmt.Sprintf("Updated %s", args[0]))
		},
	}

	configCmd.AddCommand(listCmd, getCmd, setCmd)
	return []*cobra.Command{configCmd}, nil
}
