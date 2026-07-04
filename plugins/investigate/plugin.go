package investigate

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"time"

	"gaia/kernel"
	"gaia/plugins/ask"
	"gaia/plugins/mempalace"
	"gaia/plugins/roles"
	"gaia/plugins/shared"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type InvestigatePlugin struct {
	providers map[string]ask.Provider
}

func NewInvestigatePlugin() *InvestigatePlugin {
	p := &InvestigatePlugin{
		providers: map[string]ask.Provider{},
	}
	p.RegisterProvider(ask.NewOllamaProvider())
	p.RegisterProvider(ask.NewOpenAIProvider())
	p.RegisterProvider(ask.NewMistralProvider())
	return p
}

func (p *InvestigatePlugin) ID() string           { return "investigate" }
func (p *InvestigatePlugin) DefaultEnabled() bool { return true }
func (p *InvestigatePlugin) DependsOn() []string  { return nil }
func (p *InvestigatePlugin) ConfigSchema() []string {
	return []string{
		"investigate.provider",
		"investigate.host",
		"investigate.port",
		"investigate.model",
		"investigate.timeout_seconds",
		"investigate.role",
		"investigate.max_steps",
		"investigate.max_parse_failures",
		"investigate.max_output_bytes",
		"investigate.command_timeout_seconds",
		"investigate.confirm_medium_risk",
		"investigate.denylist",
		"investigate.allowlist",
		"investigate.treat_exit_code_1_as_success",
	}
}

func (p *InvestigatePlugin) MCPTools() []kernel.MCPTool { return nil }

func (p *InvestigatePlugin) RegisterProvider(provider ask.Provider) {
	if provider == nil {
		return
	}
	p.providers[provider.Name()] = provider
}

func (p *InvestigatePlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "investigate [goal]",
		Short: "Investigate a goal using an operator loop",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			goal := strings.TrimSpace(strings.Join(args, " "))
			if goal == "" {
				return shared.PrintError(cmd.ErrOrStderr(), "Goal cannot be empty")
			}

			req := ask.AskRequest{
				Provider:    ask.FirstNonEmpty(viper.GetString("investigate.provider"), viper.GetString("provider")),
				Host:        ask.FirstNonEmpty(viper.GetString("investigate.host"), viper.GetString("host")),
				Port:        ask.FirstNonZero(viper.GetInt("investigate.port"), viper.GetInt("port")),
				Model:       ask.FirstNonEmpty(viper.GetString("investigate.model"), viper.GetString("model")),
				Timeout:     time.Duration(ask.FirstNonZero(viper.GetInt("investigate.timeout_seconds"), viper.GetInt("timeout_seconds"))) * time.Second,
				Pull:        false,
				ProgressOut: cmd.ErrOrStderr(),
			}
			if req.Timeout == 0 {
				req.Timeout = 120 * time.Second
			}
			if strings.TrimSpace(req.Provider) == "" {
				req.Provider = ask.ResolveProviderFromModel(req.Model)
			}
			if err := resolveInvestigateSystemPrompt(cmd, &req, goal); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			if pull, _ := cmd.Flags().GetBool("pull"); pull {
				req.Pull = true
			}
			if err := validateInvestigateConfig(req); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			if memCtx, err := mempalace.InjectIfEnabled(cmd.Context(), goal); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			} else if memCtx != "" {
				req.SystemPrompt = mempalace.AppendMemory(req.SystemPrompt, memCtx)
			}

			provider, ok := p.providers[req.Provider]
			if !ok {
				if fallback, hasFallback := p.providers["ollama"]; hasFallback {
					provider = fallback
				} else {
					return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Unknown provider %q", req.Provider))
				}
			}

			maxSteps := viper.GetInt("investigate.max_steps")
			if flag := cmd.Flags().Lookup("max-steps"); flag != nil && flag.Changed {
				maxSteps, _ = cmd.Flags().GetInt("max-steps")
			}
			dryRun, _ := cmd.Flags().GetBool("dry-run")
			yes, _ := cmd.Flags().GetBool("yes")
			debug, _ := cmd.Flags().GetBool("debug")

			maxParseFailures := viper.GetInt("investigate.max_parse_failures")
			maxOutputBytes := viper.GetInt("investigate.max_output_bytes")
			confirmMedium := viper.GetBool("investigate.confirm_medium_risk")
			commandTimeout := viper.GetInt("investigate.command_timeout_seconds")
			if commandTimeout <= 0 {
				commandTimeout = 30
			}
			allowlist := getStringSlice("investigate.allowlist")
			denylist := getStringSlice("investigate.denylist")
			if denylist == nil {
				denylist = defaultInvestigateDenylist
			}
			treatExit1 := viper.GetBool("investigate.treat_exit_code_1_as_success")
			if !viper.IsSet("investigate.treat_exit_code_1_as_success") {
				treatExit1 = true
			}

			runner := &shellRunnerWithTimeout{
				timeout:                 time.Duration(commandTimeout) * time.Second,
				treatExitCode1AsSuccess: treatExit1,
				in:                      cmd.InOrStdin(),
				out:                     cmd.OutOrStdout(),
			}

			sendReq := func(r Request) (string, error) {
				askReq := ask.AskRequest{
					Provider:     req.Provider,
					Host:         req.Host,
					Port:         req.Port,
					Model:        r.Model,
					Timeout:      req.Timeout,
					SystemPrompt: req.SystemPrompt,
					Pull:         req.Pull,
					ProgressOut:  cmd.ErrOrStderr(),
				}
				askReq.Messages = toChatMessages(r.Messages)
				askReq = ask.ApplySanitize(cmd.ErrOrStderr(), askReq)
				resp, err := provider.Send(cmd.Context(), askReq)
				if err != nil {
					return "", err
				}
				return resp.Text, nil
			}

			opts := RunOptions{
				MaxSteps:          maxSteps,
				DryRun:            dryRun,
				Yes:               yes,
				Debug:             debug,
				Model:             req.Model,
				Denylist:          denylist,
				Allowlist:         allowlist,
				ConfirmMediumRisk: confirmMedium,
				ConfirmFunc: func(message string) (bool, error) {
					return promptConfirm(cmd, message)
				},
				ShellRunner:      runner,
				MaxOutputBytes:   maxOutputBytes,
				MaxParseFailures: maxParseFailures,
				SendReq:          sendReq,
				Debugf: func(format string, args ...any) {
					msg := fmt.Sprintf("[DEBUG] "+format, args...)
					_ = shared.PrintRaw(cmd.ErrOrStderr(), msg)
				},
			}

			finalAnswer, err := Run(cmd.Context(), goal, opts)
			if err != nil {
				if errors.Is(err, ErrMaxStepsReached) {
					_ = shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Warning: %v", err))
				} else {
					return shared.PrintError(cmd.ErrOrStderr(), err.Error())
				}
			}
			if err := mempalace.DiaryWriteIfEnabled(cmd.Context(), goal, finalAnswer); err != nil && viper.GetBool("debug") {
				_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] mempalace diary write failed: %v\n", err))
			}
			if err := mempalace.PersistInvestigateResult(cmd.Context(), goal, finalAnswer); err != nil && viper.GetBool("debug") {
				_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] mempalace persist failed: %v\n", err))
			}

			return shared.PrintBox(cmd.OutOrStdout(), "Investigate", finalAnswer)
		},
	}

	cmd.Flags().IntP("max-steps", "n", 10, "Maximum number of operator steps")
	cmd.Flags().Bool("dry-run", false, "Do not execute commands; only show what would be run")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmation for medium-risk commands")
	cmd.Flags().Bool("debug", false, "Print debug output (decisions and observations)")
	cmd.Flags().String("role", "", "Role name to apply to the planner")
	cmd.Flags().Bool("pull", false, "Pull model from Ollama if available (force refresh)")

	_ = viper.BindPFlag("investigate.role", cmd.Flags().Lookup("role"))
	return []*cobra.Command{cmd}, nil
}

var defaultInvestigateDenylist = []string{"rm -rf", "sudo", "mkfs", "> /dev/sd"}

type shellRunnerWithTimeout struct {
	timeout                 time.Duration
	treatExitCode1AsSuccess bool
	in                      io.Reader
	out                     io.Writer
}

func (s *shellRunnerWithTimeout) Run(ctx context.Context, cmd string) (stdout, stderr string, err error) {
	if s.timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, s.timeout)
		defer cancel()
	}
	if s.in != nil && s.out != nil && shared.HasTTYStdin() && shared.HasTTYStdout() {
		decision, err := shared.RunCommandPreviewTUI(cmd, "Command", s.in, s.out)
		if err != nil {
			return "", "", err
		}
		switch decision {
		case "run":
		case "skip":
			return "", "", ErrCommandSkipped
		default:
			return "", "", ErrCommandCancelled
		}
	}
	return executeExternalCommand(ctx, cmd, s.treatExitCode1AsSuccess)
}

func executeExternalCommand(ctx context.Context, command string, treatExit1AsSuccess bool) (stdout, stderr string, err error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", "", fmt.Errorf("empty command")
	}
	// nosemgrep
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	stdoutStr := strings.TrimSpace(outBuf.String())
	stderrStr := strings.TrimSpace(errBuf.String())
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			if exitErr.ExitCode() == 1 && treatExit1AsSuccess {
				return stdoutStr, stderrStr, nil
			}
			return stdoutStr, stderrStr, fmt.Errorf("command failed with exit code %d: %w", exitErr.ExitCode(), err)
		}
		return stdoutStr, stderrStr, fmt.Errorf("failed to execute command: %w", err)
	}
	return stdoutStr, stderrStr, nil
}

func promptConfirm(cmd *cobra.Command, message string) (bool, error) {
	if !shared.HasTTYStdin() || !shared.HasTTYStdout() {
		return false, shared.PrintError(cmd.ErrOrStderr(), "No TTY available for confirmation prompt")
	}
	return shared.RunConfirmationPromptTUI(message, "Confirm", cmd.InOrStdin(), cmd.OutOrStdout())
}

func validateInvestigateConfig(req ask.AskRequest) error {
	missing := []string{}
	if strings.TrimSpace(req.Host) == "" {
		missing = append(missing, "host")
	}
	if req.Port == 0 {
		missing = append(missing, "port")
	}
	if strings.TrimSpace(req.Model) == "" {
		missing = append(missing, "model")
	}
	if req.Timeout == 0 {
		missing = append(missing, "timeout_seconds")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing investigate configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}

func getStringSlice(key string) []string {
	raw := viper.Get(key)
	if raw == nil {
		return nil
	}
	switch v := raw.(type) {
	case []string:
		return v
	case []interface{}:
		out := make([]string, 0, len(v))
		for _, item := range v {
			if s, ok := item.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func resolveInvestigateSystemPrompt(cmd *cobra.Command, req *ask.AskRequest, goal string) error {
	ctxPrompt, err := mempalace.SearchContextIfEnabled(cmd.Context(), goal)
	if err != nil {
		return err
	}
	if ctxPrompt != "" {
		req.SystemPrompt = ctxPrompt
		return nil
	}
	return applyInvestigateRole(cmd, req, goal)
}

func applyInvestigateRole(cmd *cobra.Command, req *ask.AskRequest, goal string) error {
	roleName := strings.TrimSpace(viper.GetString("investigate.role"))
	if roleName == "" && viper.GetBool("roles.auto_select") {
		kw := roles.LoadKeywordConfig()
		weight := viper.GetFloat64("roles.scoring.weight")
		if weight == 0 {
			weight = 1.0
		}
		threshold := viper.GetFloat64("roles.scoring.min_threshold")
		defaultRole := viper.GetString("roles.default_role")
		res := roles.SelectRoleForText(goal, kw, weight, threshold, defaultRole)
		roleName = res.RoleName
		if viper.GetBool("roles.debug") {
			roles.SetDebugWriter(cmd.ErrOrStderr())
			roles.LogScores(res.AllScores, res.Threshold, res.RoleName)
		}
	}
	if roleName == "" {
		return nil
	}
	rolesList, err := roles.LoadRolesWithDefaults()
	if err != nil {
		return err
	}
	resolved, err := roles.ResolveInheritance(rolesList)
	if err != nil {
		return err
	}
	role, ok := resolved[roleName]
	if !ok {
		return fmt.Errorf("role %q not found", roleName)
	}
	req.SystemPrompt = roles.ResolveSystemPrompt(role, req.Provider, req.Model)
	return nil
}

func toChatMessages(messages []Message) []ask.ChatMessage {
	out := make([]ask.ChatMessage, 0, len(messages))
	for _, m := range messages {
		if strings.TrimSpace(m.Role) == "" || strings.TrimSpace(m.Content) == "" {
			continue
		}
		out = append(out, ask.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return out
}

