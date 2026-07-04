package tools

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"

	"gaia/config"
	"gaia/kernel"
	"gaia/plugins/ask"
	"gaia/plugins/mempalace"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ToolsPlugin struct {
	providers map[string]ask.Provider
}

func NewToolsPlugin() *ToolsPlugin {
	p := &ToolsPlugin{
		providers: map[string]ask.Provider{},
	}
	p.providers["ollama"] = ask.NewOllamaProvider()
	p.providers["openai"] = ask.NewOpenAIProvider()
	p.providers["mistral"] = ask.NewMistralProvider()
	return p
}

func (p *ToolsPlugin) ID() string           { return "tools" }
func (p *ToolsPlugin) DefaultEnabled() bool { return true }
func (p *ToolsPlugin) DependsOn() []string  { return nil }
func (p *ToolsPlugin) ConfigSchema() []string {
	return []string{
		"tools.allow",
		"tools.allow_patterns",
		"tools.deny",
		"tools.deny_patterns",
		"tools.*",
	}
}

func (p *ToolsPlugin) MCPTools() []kernel.MCPTool { return nil }

func (p *ToolsPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "tool",
		Short: "Run external tools with approval",
		Args:  cobra.MinimumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			tool := args[0]
			action := args[1]
			actionArgs := []string{}
			if len(args) > 2 {
				actionArgs = args[2:]
			}
			pull, _ := cmd.Flags().GetBool("pull")
			if err := runToolAction(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), cmd.InOrStdin(), tool, action, actionArgs, p.providers, pull); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			return nil
		},
	}
	root.Flags().Bool("pull", false, "Pull model from Ollama if available (force refresh)")

	runCmd := &cobra.Command{
		Use:   "run [command] [args...]",
		Short: "Run a command with approval",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			command := args[0]
			subcommand := ""
			if len(args) > 1 && !strings.HasPrefix(args[1], "-") {
				subcommand = args[1]
			}
			key := command
			if subcommand != "" {
				key = command + " " + subcommand
			}

			allowed := viper.GetStringSlice("tools.allow")
			denied := viper.GetStringSlice("tools.deny")
			allowPatterns := viper.GetStringSlice("tools.allow_patterns")
			denyPatterns := viper.GetStringSlice("tools.deny_patterns")

			if matchExact(denied, key) || matchPattern(denyPatterns, key) {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Command denied: %s", key))
			}
			if matchExact(allowed, key) || matchPattern(allowPatterns, key) {
				return runCommand(cmd.Context(), command, args[1:], cmd)
			}

			for {
				decision, pattern, newKey, err := promptDecision(cmd, key, command, subcommand)
				if err != nil {
					return err
				}
				if decision == "edit" {
					if newKey != "" {
						key = newKey
					}
					continue
				}
				if decision == "cancel" {
					return shared.PrintError(cmd.ErrOrStderr(), "Cancelled")
				}
				if decision == "deny_exact" {
					denied = appendUnique(denied, key)
					if err := persistList("tools.deny", denied); err != nil {
						return err
					}
					return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Command denied: %s", key))
				}
				if decision == "deny_pattern" {
					denyPatterns = appendUnique(denyPatterns, pattern)
					if err := persistList("tools.deny_patterns", denyPatterns); err != nil {
						return err
					}
					return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Command denied: %s", key))
				}
				if decision == "allow_exact" {
					allowed = appendUnique(allowed, key)
					if err := persistList("tools.allow", allowed); err != nil {
						return err
					}
					return runCommand(cmd.Context(), command, args[1:], cmd)
				}
				if decision == "allow_pattern" {
					allowPatterns = appendUnique(allowPatterns, pattern)
					if err := persistList("tools.allow_patterns", allowPatterns); err != nil {
						return err
					}
					return runCommand(cmd.Context(), command, args[1:], cmd)
				}
				return shared.PrintError(cmd.ErrOrStderr(), "Unknown decision")
			}

		},
	}

	root.AddCommand(runCmd)
	return []*cobra.Command{root}, nil
}

func promptDecision(cmd *cobra.Command, key, command, subcommand string) (string, string, string, error) {
	if !shared.HasTTYStdin() || !shared.HasTTYStdout() {
		return "cancel", "", "", shared.PrintError(cmd.ErrOrStderr(), "No TTY available for approval prompt")
	}

	patternDefault := command
	if subcommand != "" {
		patternDefault = command + " *"
	}

	return shared.RunApprovalPromptTUI(key, patternDefault, key, cmd.InOrStdin(), cmd.OutOrStdout())
}

func runCommand(ctx context.Context, command string, args []string, cmd *cobra.Command) error {
	full := command
	if len(args) > 0 {
		full = full + " " + strings.Join(args, " ")
	}
	if shared.HasTTYStdin() && shared.HasTTYStdout() {
		decision, err := shared.RunCommandPreviewTUI(full, "Command", cmd.InOrStdin(), cmd.OutOrStdout())
		if err != nil {
			return err
		}
		switch decision {
		case "run":
		case "skip":
			return nil
		default:
			return fmt.Errorf("command cancelled")
		}
	}
	// nosemgrep
	c := exec.CommandContext(ctx, command, args...)
	c.Stdout = cmd.OutOrStdout()
	c.Stderr = cmd.ErrOrStderr()
	c.Stdin = cmd.InOrStdin()
	err := c.Run()
	if err != nil {
		exitCode := extractExitCode(err)
		persistErr := mempalace.PersistToolExecution(ctx, full, fmt.Sprintf("error: %v", err), exitCode)
		if persistErr != nil {
			return fmt.Errorf("%w; mempalace add drawer failed: %v", err, persistErr)
		}
		return err
	}
	if err := mempalace.PersistToolExecution(ctx, full, "success", 0); err != nil {
		return fmt.Errorf("mempalace add drawer failed: %v", err)
	}
	return nil
}

func extractExitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func matchExact(list []string, key string) bool {
	for _, item := range list {
		if item == key {
			return true
		}
	}
	return false
}

func matchPattern(patterns []string, key string) bool {
	for _, pattern := range patterns {
		pattern = strings.TrimSpace(pattern)
		if pattern == "" {
			continue
		}
		if ok, _ := pathMatch(pattern, key); ok {
			return true
		}
	}
	return false
}

func pathMatch(pattern, value string) (bool, error) {
	pattern = strings.ReplaceAll(pattern, "**", "*")
	return pathMatchImpl(pattern, value), nil
}

func pathMatchImpl(pattern, value string) bool {
	parts := strings.Split(pattern, "*")
	if len(parts) == 1 {
		return pattern == value
	}
	if !strings.HasPrefix(value, parts[0]) {
		return false
	}
	value = value[len(parts[0]):]
	for i := 1; i < len(parts); i++ {
		part := parts[i]
		if part == "" {
			continue
		}
		idx := strings.Index(value, part)
		if idx == -1 {
			return false
		}
		value = value[idx+len(part):]
	}
	return true
}

func appendUnique(list []string, value string) []string {
	for _, item := range list {
		if item == value {
			return list
		}
	}
	return append(list, value)
}

func persistList(key string, list []string) error {
	quoted := make([]string, 0, len(list))
	for _, v := range list {
		quoted = append(quoted, fmt.Sprintf("%q", v))
	}
	return config.SetConfigString(key, "["+strings.Join(quoted, ",")+"]")
}
