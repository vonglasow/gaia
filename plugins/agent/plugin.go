package agent

import (
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gaia/kernel"
	"gaia/plugins/ask"
	"gaia/plugins/shared"
)

// AgentPlugin exposes `gaia agent`: a model on a project, under a policy, in a workspace.
type AgentPlugin struct {
	kernel.BasePlugin

	providers map[string]ask.Provider
}

func NewAgentPlugin() *AgentPlugin {
	p := &AgentPlugin{providers: map[string]ask.Provider{}}
	p.RegisterProvider(ask.NewOllamaProvider())
	p.RegisterProvider(ask.NewOpenAIProvider())
	p.RegisterProvider(ask.NewMistralProvider())
	return p
}

func (p *AgentPlugin) ID() string           { return "agent" }
func (p *AgentPlugin) DefaultEnabled() bool { return true }

func (p *AgentPlugin) ConfigSchema() []string {
	return []string{
		"agent.provider",
		"agent.host",
		"agent.port",
		"agent.model",
		"agent.timeout_seconds",
		"agent.max_steps",
		"agent.role",
		"agent.command_timeout_seconds",
		"agent.max_file_bytes",
		// Added to the built-in policy, never replacing it.
		"agent.allowlist",
		"agent.denylist",
	}
}

// RegisterProvider ignores a nil backend, which would otherwise dereference later.
func (p *AgentPlugin) RegisterProvider(provider ask.Provider) {
	if provider == nil {
		return
	}
	p.providers[provider.Name()] = provider
}

func (p *AgentPlugin) Register(_ *kernel.Kernel) ([]*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "agent [task]",
		Short: "Work on a project with a model: read, plan, change, check",
		Long: "Give a model the project and a task. It reads files, runs the project's own\n" +
			"checks, and reports what it found. It changes nothing unless --write says so.\n\n" +
			"Every path it touches is inside the working directory, every command it runs\n" +
			"goes through the same policy as `gaia investigate`, and the loop stops on its\n" +
			"own whether or not the model thinks it is finished.",
		Args: cobra.MinimumNArgs(1),
		RunE: p.run,
	}

	ask.AddModelFlags(cmd, "agent")
	cmd.Flags().Bool("write", false, "Let the agent change files (off by default: it reads, runs checks and explains)")
	cmd.Flags().IntP("max-steps", "n", 0, "Stop after this many turns (default 20)")
	cmd.Flags().StringP("dir", "C", ".", "Project to work on")
	cmd.Flags().BoolP("yes", "y", false, "Run commands that would otherwise be confirmed")
	cmd.Flags().Bool("quiet", false, "Only print the final answer")
	cmd.Flags().Bool("unload", false, "Drop the model from memory when the run ends")

	return []*cobra.Command{cmd}, nil
}

func (p *AgentPlugin) run(cmd *cobra.Command, args []string) error {
	task := strings.TrimSpace(strings.Join(args, " "))
	if task == "" {
		return shared.Fail(cmd.ErrOrStderr(), "There is no task to work on.")
	}

	dir, _ := cmd.Flags().GetString("dir")
	ws, err := NewWorkspace(dir)
	if err != nil {
		return shared.Fail(cmd.ErrOrStderr(), err.Error())
	}

	allowWrites, _ := cmd.Flags().GetBool("write")
	quiet, _ := cmd.Flags().GetBool("quiet")
	unload, _ := cmd.Flags().GetBool("unload")

	perms := PermissionsFor(cmd, "agent")
	perms.AllowWrites = allowWrites
	tools := NewToolset(ws, perms)

	req, err := p.request(cmd)
	if err != nil {
		return shared.Fail(cmd.ErrOrStderr(), err.Error())
	}
	provider, err := ask.ProviderFor(p.providers, req.Provider)
	if err != nil {
		return shared.Fail(cmd.ErrOrStderr(), err.Error())
	}

	systemPrompt, err := p.systemPrompt(cmd, ws, tools, allowWrites)
	if err != nil {
		return shared.Fail(cmd.ErrOrStderr(), err.Error())
	}

	maxSteps, _ := cmd.Flags().GetInt("max-steps")

	if !quiet {
		_ = shared.PrintBox(cmd.OutOrStdout(), "Agent", fmt.Sprintf(
			"Project: %s\nModel: %s\nWrites: %s\nTools: %s",
			ws.Root(), req.Model, writeMode(allowWrites), strings.Join(tools.Names(), ", ")))
	}

	return Work{
		Plugin: "agent", Title: "Agent", Task: task,
		Tools: tools, Prompt: systemPrompt, Request: req, Provider: provider,
		MaxSteps: maxSteps, Quiet: quiet, Unload: unload, Writing: allowWrites,
		Render: renderStep,
	}.Do(cmd)
}

func renderStep(step Step) string {
	var b strings.Builder
	if step.Recovered {
		// Worth seeing: the model is not calling tools properly, only nearly.
		fmt.Fprintf(&b, "     (read a tool call out of the model's text)\n")
	}
	for i, call := range step.ToolCalls {
		// Once per turn: three calls printed "4." three times and read as three turns.
		if i == 0 {
			fmt.Fprintf(&b, "  %d. %s", step.Number, call.Name)
		} else {
			fmt.Fprintf(&b, "     %s", call.Name)
		}
		if len(call.Arguments) > 0 {
			for _, key := range sortedKeys(call.Arguments) {
				value, _ := call.ArgString(key)
				fmt.Fprintf(&b, " %s=%s", key, firstLine(value, 60))
			}
		}
		if i < len(step.Results) {
			fmt.Fprintf(&b, " → %s", firstLine(step.Results[i], 80))
		}
		b.WriteString("\n")
	}
	return b.String()
}

// firstLine keeps a trace to one line, so a file read does not bury it.
func firstLine(s string, width int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i] + " …"
	}
	if len([]rune(s)) > width {
		s = string([]rune(s)[:width]) + "…"
	}
	return s
}

func writeMode(allowed bool) string {
	if allowed {
		return "allowed"
	}
	return "refused (pass --write to change files)"
}

// request resolves the model for a terminal run, where flags win.
func (p *AgentPlugin) request(cmd *cobra.Command) (ask.AskRequest, error) {
	model, _ := cmd.Flags().GetString("model")
	provider, _ := cmd.Flags().GetString("provider")
	return ask.Endpoint{
		Plugin: "agent", Model: model, Provider: provider,
		// Longer than a single question: reasoning over a file takes longer.
		Timeout: 5 * time.Minute,
	}.Resolve()
}
