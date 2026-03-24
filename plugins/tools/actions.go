package tools

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sort"
	"strings"
	"time"

	"gaia/plugins/ask"
	"gaia/plugins/roles"
	"gaia/plugins/shared"
	sanitizepkg "gaia/plugins/shared/sanitize"

	"github.com/spf13/viper"
)

type toolActionConfig struct {
	ContextCommand string
	ExecuteCommand string
	Role           string
	Provider       string
	Host           string
	Port           int
	Model          string
	TimeoutSeconds int
}

func loadToolActionConfig(tool, action string) toolActionConfig {
	prefix := fmt.Sprintf("tools.%s.%s.", tool, action)
	return toolActionConfig{
		ContextCommand: viper.GetString(prefix + "context_command"),
		ExecuteCommand: viper.GetString(prefix + "execute_command"),
		Role:           viper.GetString(prefix + "role"),
		Provider:       viper.GetString(prefix + "provider"),
		Host:           viper.GetString(prefix + "host"),
		Port:           viper.GetInt(prefix + "port"),
		Model:          viper.GetString(prefix + "model"),
		TimeoutSeconds: viper.GetInt(prefix + "timeout_seconds"),
	}
}

func runToolAction(ctx context.Context, out io.Writer, errOut io.Writer, in io.Reader, tool, action string, args []string, providers map[string]ask.Provider, pull bool) error {
	cfg := loadToolActionConfig(tool, action)
	if strings.TrimSpace(cfg.ContextCommand) == "" && strings.TrimSpace(cfg.ExecuteCommand) == "" {
		return fmt.Errorf("tool %q action %q has no context_command or execute_command configured", tool, action)
	}

	contextOut := ""
	if strings.TrimSpace(cfg.ContextCommand) != "" {
		stdout, stderr, err := executeShell(ctx, cfg.ContextCommand)
		if err != nil {
			if stderr != "" {
				return fmt.Errorf("context_command failed: %w (stderr: %s)", err, stderr)
			}
			return fmt.Errorf("context_command failed: %w", err)
		}
		contextOut = strings.TrimSpace(stdout)
	}

	req := ask.AskRequest{
		Provider:    ask.FirstNonEmpty(cfg.Provider, viper.GetString("provider")),
		Host:        ask.FirstNonEmpty(cfg.Host, viper.GetString("host")),
		Port:        ask.FirstNonZero(cfg.Port, viper.GetInt("port")),
		Model:       ask.FirstNonEmpty(cfg.Model, viper.GetString("model")),
		Timeout:     time.Duration(ask.FirstNonZero(cfg.TimeoutSeconds, viper.GetInt("timeout_seconds"))) * time.Second,
		Pull:        pull,
		ProgressOut: errOut,
	}
	if req.Timeout == 0 {
		req.Timeout = 120 * time.Second
	}
	if strings.TrimSpace(req.Provider) == "" {
		req.Provider = ask.ResolveProviderFromModel(req.Model)
	}

	provider, ok := providers[req.Provider]
	if !ok {
		if fallback, hasFallback := providers["ollama"]; hasFallback {
			provider = fallback
		} else {
			return fmt.Errorf("unknown provider %q", req.Provider)
		}
	}

	rolePrompt, err := resolveToolRolePrompt(cfg.Role, tool, action, contextOut, req)
	if err != nil {
		return err
	}
	req.SystemPrompt = rolePrompt
	req.Message = buildToolPrompt(tool, action, args, contextOut)

	req = applySanitize(req, errOut)
	resp, err := provider.Send(ctx, req)
	if err != nil {
		return err
	}
	response := strings.TrimSpace(resp.Text)
	if response == "" {
		return fmt.Errorf("tool action returned empty response")
	}

	if strings.TrimSpace(cfg.ExecuteCommand) == "" {
		return shared.PrintBox(out, "Response", response)
	}
	return executeToolCommand(ctx, cfg.ExecuteCommand, response)
}

func resolveToolRolePrompt(roleName, tool, action, contextOut string, req ask.AskRequest) (string, error) {
	roleName = strings.TrimSpace(roleName)
	if roleName == "" && viper.GetBool("roles.auto_select") {
		kw := loadRoleKeywords()
		weight := viper.GetFloat64("roles.scoring.weight")
		if weight == 0 {
			weight = 1.0
		}
		threshold := viper.GetFloat64("roles.scoring.min_threshold")
		defaultRole := viper.GetString("roles.default_role")
		text := fmt.Sprintf("%s %s %s", tool, action, contextOut)
		res := roles.SelectRoleForText(text, kw, weight, threshold, defaultRole)
		roleName = res.RoleName
		if viper.GetBool("roles.debug") {
			roles.SetDebugWriter(os.Stderr)
			roles.LogScores(res.AllScores, res.Threshold, res.RoleName)
		}
	}
	if roleName == "" {
		return "", nil
	}
	rolesList, err := loadRolesFromConfig()
	if err != nil {
		return "", err
	}
	resolved, err := roles.ResolveInheritance(rolesList)
	if err != nil {
		return "", err
	}
	role, ok := resolved[roleName]
	if !ok {
		return "", fmt.Errorf("role %q not found", roleName)
	}
	return roles.ResolveSystemPrompt(role, req.Provider, req.Model), nil
}

func buildToolPrompt(tool, action string, args []string, contextOut string) string {
	var b strings.Builder
	b.WriteString(fmt.Sprintf("Tool: %s\nAction: %s\n", tool, action))
	if len(args) > 0 {
		b.WriteString("Args: ")
		b.WriteString(strings.Join(args, " "))
		b.WriteString("\n")
	}
	if contextOut != "" {
		b.WriteString("Context:\n")
		b.WriteString(contextOut)
		b.WriteString("\n")
	}
	b.WriteString("Respond with the best output for this action.")
	return b.String()
}

func executeToolCommand(ctx context.Context, template string, response string) error {
	responseTrimmed := strings.TrimSpace(response)
	tmpFile, err := os.CreateTemp("", "gaia-tool-*")
	if err != nil {
		return err
	}
	defer func() {
		_ = os.Remove(tmpFile.Name())
	}()
	if _, err := tmpFile.WriteString(responseTrimmed); err != nil {
		_ = tmpFile.Close()
		return err
	}
	_ = tmpFile.Close()

	replacements := map[string]string{
		"{response}": responseTrimmed,
		"{output}":   responseTrimmed,
		"{file}":     tmpFile.Name(),
	}

	if requiresShell(template) {
		cmdStr := replaceAll(template, replacements)
		return runShell(ctx, cmdStr)
	}
	name, args, err := buildCommandArgs(template, replacements)
	if err != nil {
		return err
	}
	// nosemgrep
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	cmd.Stdin = os.Stdin
	return cmd.Run()
}

func executeShell(ctx context.Context, command string) (stdout, stderr string, err error) {
	command = strings.TrimSpace(command)
	if command == "" {
		return "", "", fmt.Errorf("empty command")
	}
	// nosemgrep
	cmd := exec.CommandContext(ctx, "sh", "-c", command)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err = cmd.Run()
	return strings.TrimSpace(outBuf.String()), strings.TrimSpace(errBuf.String()), err
}

func runShell(ctx context.Context, command string) error {
	_, _, err := executeShell(ctx, command)
	return err
}

func replaceAll(template string, replacements map[string]string) string {
	out := template
	for k, v := range replacements {
		out = strings.ReplaceAll(out, k, v)
	}
	return out
}

func requiresShell(cmd string) bool {
	return strings.Contains(cmd, "|") || strings.Contains(cmd, ">") || strings.Contains(cmd, "<") || strings.Contains(cmd, "&&") || strings.Contains(cmd, "||")
}

// buildCommandArgs parses a template like "git checkout -b {response}" with replacements
// and returns (executable, args) so each placeholder value is exactly one argument (no shell).
func buildCommandArgs(template string, replacements map[string]string) (name string, args []string, err error) {
	type seg struct {
		start int
		end   int
		key   string
	}
	var segments []seg
	placeholders := []string{"{file}", "{response}", "{output}"}
	for _, p := range placeholders {
		idx := strings.Index(template, p)
		for idx != -1 {
			segments = append(segments, seg{start: idx, end: idx + len(p), key: p})
			next := strings.Index(template[idx+len(p):], p)
			if next == -1 {
				break
			}
			idx = idx + len(p) + next
		}
	}
	if len(segments) == 0 {
		parts := strings.Fields(template)
		if len(parts) == 0 {
			return "", nil, fmt.Errorf("invalid execute_command: empty")
		}
		return parts[0], parts[1:], nil
	}
	sort.Slice(segments, func(i, j int) bool {
		return segments[i].start < segments[j].start
	})
	var parts []string
	last := 0
	for _, s := range segments {
		if s.start > last {
			literal := strings.TrimSpace(template[last:s.start])
			if literal != "" {
				parts = append(parts, strings.Fields(literal)...)
			}
		}
		parts = append(parts, replacements[s.key])
		last = s.end
	}
	if last < len(template) {
		literal := strings.TrimSpace(template[last:])
		if literal != "" {
			parts = append(parts, strings.Fields(literal)...)
		}
	}
	if len(parts) == 0 {
		return "", nil, fmt.Errorf("invalid execute_command: empty")
	}
	return parts[0], parts[1:], nil
}

func loadRolesFromConfig() ([]roles.Role, error) {
	dir := strings.TrimSpace(viper.GetString("roles.directory"))
	if dir == "" {
		defaultDir, err := roles.DefaultRolesDir()
		if err != nil {
			return nil, err
		}
		dir = defaultDir
	}
	if err := roles.EnsureRolesDir(dir); err != nil {
		return nil, err
	}
	return roles.LoadRoles(dir)
}

func loadRoleKeywords() map[string][]string {
	out := map[string][]string{}
	for _, key := range viper.AllKeys() {
		if !strings.HasPrefix(key, "roles.keywords.") {
			continue
		}
		name := strings.TrimPrefix(key, "roles.keywords.")
		if name == "" {
			continue
		}
		out[name] = viper.GetStringSlice(key)
	}
	return out
}

func applySanitize(req ask.AskRequest, errOut io.Writer) ask.AskRequest {
	if !viper.GetBool("sanitize.enabled") {
		return req
	}
	levelStr := strings.ToLower(strings.TrimSpace(viper.GetString("sanitize.level")))
	var level sanitizepkg.Level
	switch levelStr {
	case "none":
		level = sanitizepkg.LevelNone
	case "aggressive":
		level = sanitizepkg.LevelAggressive
	case "light", "":
		level = sanitizepkg.LevelLight
	default:
		level = sanitizepkg.LevelLight
	}
	opts := sanitizepkg.Options{
		Level:             level,
		MaxTokensAfter:    viper.GetInt("sanitize.max_tokens_after"),
		LogStats:          viper.GetBool("sanitize.log_stats"),
		PreserveLastUser:  true,
		MaxDurationMillis: 100,
	}
	raw := buildMessagesForSanitize(req)
	sreq := sanitizepkg.Request{Messages: raw}
	out, stats, err := sanitizepkg.Sanitize(sreq, opts)
	if err != nil {
		if viper.GetBool("debug") {
			_, _ = fmt.Fprintf(errOut, "[DEBUG] sanitize: %v\n", err)
		}
		return req
	}
	if opts.LogStats && (stats.TokensBefore > 0 || stats.TokensAfter > 0) {
		_, _ = fmt.Fprintf(errOut, "[sanitize] tokens before=%d after=%d removed≈%d ms=%d\n",
			stats.TokensBefore, stats.TokensAfter, stats.RemovedCount, stats.DurationMillis)
	}
	req.SystemPrompt = ""
	req.Message = ""
	req.Messages = make([]ask.ChatMessage, 0, len(out.Messages))
	for _, m := range out.Messages {
		req.Messages = append(req.Messages, ask.ChatMessage{Role: m.Role, Content: m.Content})
	}
	return req
}

func buildMessagesForSanitize(req ask.AskRequest) []sanitizepkg.Message {
	out := []sanitizepkg.Message{}
	if strings.TrimSpace(req.SystemPrompt) != "" {
		out = append(out, sanitizepkg.Message{Role: "system", Content: req.SystemPrompt})
	}
	for _, m := range req.Messages {
		if strings.TrimSpace(m.Role) == "" || strings.TrimSpace(m.Content) == "" {
			continue
		}
		out = append(out, sanitizepkg.Message{Role: m.Role, Content: m.Content})
	}
	if strings.TrimSpace(req.Message) != "" {
		out = append(out, sanitizepkg.Message{Role: "user", Content: req.Message})
	}
	return out
}
