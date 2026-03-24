package plugins

import (
	"fmt"
	"sort"
	"strings"

	"gaia/config"
	"gaia/kernel"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type PluginsPlugin struct{}

func NewPluginsPlugin() *PluginsPlugin { return &PluginsPlugin{} }

func (p *PluginsPlugin) ID() string           { return "plugins" }
func (p *PluginsPlugin) DefaultEnabled() bool { return true }
func (p *PluginsPlugin) DependsOn() []string  { return nil }
func (p *PluginsPlugin) ConfigSchema() []string {
	return nil
}

func (p *PluginsPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "plugins",
		Short: "Manage plugins",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List plugins and status",
		RunE: func(cmd *cobra.Command, args []string) error {
			plugins := k.Plugins()
			enabled := map[string]bool{}
			for _, p := range k.EnabledPlugins() {
				enabled[p.ID()] = true
			}
			var b strings.Builder
			for _, p := range plugins {
				status := "disabled"
				if enabled[p.ID()] {
					status = "enabled"
				}
				b.WriteString(fmt.Sprintf("%s\t%s\tdefault=%t\n", p.ID(), status, p.DefaultEnabled()))
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Plugins", strings.TrimRight(b.String(), "\n"))
		},
	}

	enableCmd := &cobra.Command{
		Use:   "enable [plugin]",
		Short: "Enable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if _, ok := k.Plugin(id); !ok {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Unknown plugin %q", id))
			}
			enabled := uniqueAppend(viper.GetStringSlice("plugins.enabled"), id)
			disabled := removeValue(viper.GetStringSlice("plugins.disabled"), id)
			if err := config.SetConfigString("plugins.enabled", toJSONList(enabled)); err != nil {
				return err
			}
			if err := config.SetConfigString("plugins.disabled", toJSONList(disabled)); err != nil {
				return err
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Plugins", fmt.Sprintf("Enabled %s", id))
		},
	}

	disableCmd := &cobra.Command{
		Use:   "disable [plugin]",
		Short: "Disable a plugin",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			id := args[0]
			if _, ok := k.Plugin(id); !ok {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Unknown plugin %q", id))
			}
			disabled := uniqueAppend(viper.GetStringSlice("plugins.disabled"), id)
			enabled := removeValue(viper.GetStringSlice("plugins.enabled"), id)
			if err := config.SetConfigString("plugins.enabled", toJSONList(enabled)); err != nil {
				return err
			}
			if err := config.SetConfigString("plugins.disabled", toJSONList(disabled)); err != nil {
				return err
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Plugins", fmt.Sprintf("Disabled %s", id))
		},
	}

	root.AddCommand(listCmd, enableCmd, disableCmd)
	return []*cobra.Command{root}, nil
}

func uniqueAppend(list []string, value string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(list)+1)
	for _, v := range list {
		if !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	if value != "" && !seen[value] {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func removeValue(list []string, value string) []string {
	out := make([]string, 0, len(list))
	for _, v := range list {
		if v != value {
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}

func toJSONList(values []string) string {
	quoted := make([]string, 0, len(values))
	for _, v := range values {
		quoted = append(quoted, fmt.Sprintf("%q", v))
	}
	return "[" + strings.Join(quoted, ",") + "]"
}
