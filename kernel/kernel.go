package kernel

import (
	"errors"
	"fmt"
	"log"
	"os"
	"sort"
	"strings"

	"gaia/config"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

// Kernel is the core runtime that manages plugins and the root CLI.
type Kernel struct {
	RootCmd *cobra.Command
	logger  *log.Logger
	plugins map[string]Plugin
	enabled map[string]Plugin
}

// NewKernel creates a kernel with a root command and plugin manager commands.
func NewKernel() *Kernel {
	k := &Kernel{
		logger:  log.New(os.Stderr, "[kernel] ", log.LstdFlags),
		plugins: make(map[string]Plugin),
		enabled: make(map[string]Plugin),
	}
	k.RootCmd = &cobra.Command{
		Use:   "gaia",
		Short: "Gaia CLI",
	}
	k.RootCmd.PersistentFlags().StringVarP(&config.CfgFile, "config", "c", "", "Path to an alternative YAML configuration file (or $GAIA_CONFIG)")
	k.RootCmd.PersistentFlags().Bool("debug", false, "Enable debug output (includes roles debug)")
	_ = viper.BindPFlag("debug", k.RootCmd.PersistentFlags().Lookup("debug"))
	_ = viper.BindPFlag("roles.debug", k.RootCmd.PersistentFlags().Lookup("debug"))
	return k
}

// Execute loads config, resolves plugins, registers commands, and executes the root command.
func (k *Kernel) Execute(args []string) error {
	if cfg := DetectConfigPath(args); cfg != "" {
		config.CfgFile = cfg
	}
	if err := config.InitConfig(); err != nil {
		return fmt.Errorf("init config: %w", err)
	}
	if err := k.LoadPluginConfigs(); err != nil {
		return err
	}
	if err := k.ValidateConfigKeys(); err != nil {
		return err
	}
	if err := k.ResolveEnabled(); err != nil {
		return err
	}
	if err := k.RegisterEnabledCommands(); err != nil {
		return err
	}
	k.RootCmd.SetArgs(args)
	return k.RootCmd.Execute()
}

// DetectConfigPath scans args for --config/-c and returns its value if present.
func DetectConfigPath(args []string) string {
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--config" || arg == "-c" {
			if i+1 < len(args) {
				return args[i+1]
			}
			continue
		}
		if strings.HasPrefix(arg, "--config=") {
			return strings.TrimPrefix(arg, "--config=")
		}
		if strings.HasPrefix(arg, "-c=") {
			return strings.TrimPrefix(arg, "-c=")
		}
	}
	return ""
}

// RegisterPlugin registers a built-in plugin and its config schema.
func (k *Kernel) RegisterPlugin(p Plugin) error {
	if p == nil {
		return fmt.Errorf("nil plugin")
	}
	id := strings.TrimSpace(p.ID())
	if id == "" {
		return fmt.Errorf("plugin id cannot be empty")
	}
	if _, exists := k.plugins[id]; exists {
		return fmt.Errorf("plugin %q already registered", id)
	}
	k.plugins[id] = p
	if err := config.RegisterPluginSchema(id, p.ConfigSchema()); err != nil {
		return fmt.Errorf("register plugin schema for %q: %w", id, err)
	}
	return nil
}

// Plugins returns all registered plugins.
func (k *Kernel) Plugins() []Plugin {
	out := make([]Plugin, 0, len(k.plugins))
	for _, p := range k.plugins {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// EnabledPlugins returns enabled plugins.
func (k *Kernel) EnabledPlugins() []Plugin {
	out := make([]Plugin, 0, len(k.enabled))
	for _, p := range k.enabled {
		out = append(out, p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID() < out[j].ID() })
	return out
}

// IsEnabled returns whether a plugin is enabled.
func (k *Kernel) IsEnabled(id string) bool {
	_, ok := k.enabled[id]
	return ok
}

// Plugin returns a registered plugin by id.
func (k *Kernel) Plugin(id string) (Plugin, bool) {
	p, ok := k.plugins[id]
	return p, ok
}

// ResolveEnabled computes enabled plugins based on defaults and config.
func (k *Kernel) ResolveEnabled() error {
	enabled := make(map[string]Plugin)
	for id, p := range k.plugins {
		if p.DefaultEnabled() {
			enabled[id] = p
		}
	}
	for _, id := range configList("plugins.enabled") {
		if p, ok := k.plugins[id]; ok {
			enabled[id] = p
		}
	}
	for _, id := range configList("plugins.disabled") {
		delete(enabled, id)
	}

	// Validate dependencies
	for id, p := range enabled {
		for _, dep := range p.DependsOn() {
			if dep == "" {
				continue
			}
			if _, ok := enabled[dep]; !ok {
				return fmt.Errorf("plugin %q requires %q to be enabled", id, dep)
			}
		}
	}
	k.enabled = enabled
	return nil
}

// RegisterEnabledCommands registers enabled plugins' commands.
func (k *Kernel) RegisterEnabledCommands() error {
	for _, p := range k.EnabledPlugins() {
		cmds, err := p.Register(k)
		if err != nil {
			return fmt.Errorf("register plugin %q: %w", p.ID(), err)
		}
		for _, cmd := range cmds {
			if cmd == nil {
				continue
			}
			k.RootCmd.AddCommand(cmd)
		}
	}
	return nil
}

func configList(key string) []string {
	val := viper.Get(key)
	if val == nil {
		return nil
	}
	if s, ok := val.([]string); ok {
		return s
	}
	if s, ok := val.([]interface{}); ok {
		out := make([]string, 0, len(s))
		for _, x := range s {
			if str, ok := x.(string); ok {
				out = append(out, str)
			}
		}
		return out
	}
	return nil
}

// ValidateConfigKeys checks that all config keys are allowed by plugin schemas.
func (k *Kernel) ValidateConfigKeys() error {
	keys, err := config.KeysFromFile(config.CfgFile)
	if err != nil {
		return fmt.Errorf("read config keys: %w", err)
	}
	for _, p := range k.Plugins() {
		pluginKeys, err := config.PluginConfigKeys(p.ID())
		if err != nil {
			return fmt.Errorf("read plugin config keys for %q: %w", p.ID(), err)
		}
		keys = append(keys, pluginKeys...)
	}
	mode := config.ValidationMode()
	if mode == "off" {
		return nil
	}
	invalid := []string{}
	for _, key := range keys {
		if !config.IsValidKey(key) {
			invalid = append(invalid, key)
		}
	}
	if len(invalid) == 0 {
		return nil
	}
	sort.Strings(invalid)
	msg := fmt.Sprintf("invalid config keys: %s", strings.Join(invalid, ", "))
	if mode == "warn" {
		k.logger.Printf("warning: %s", msg)
		return nil
	}
	return errors.New(msg)
}

// LoadPluginConfigs merges plugin config files into Viper.
func (k *Kernel) LoadPluginConfigs() error {
	for _, p := range k.Plugins() {
		if err := config.LoadPluginConfig(p.ID()); err != nil {
			return fmt.Errorf("load plugin config for %q: %w", p.ID(), err)
		}
	}
	return nil
}
