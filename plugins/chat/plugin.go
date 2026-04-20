package chat

import (
	"bufio"
	"fmt"
	"io"
	"strings"
	"time"

	"gaia/kernel"
	"gaia/plugins/ask"
	"gaia/plugins/cache"
	"gaia/plugins/mempalace"
	"gaia/plugins/roles"
	"gaia/plugins/shared"
	sanitizepkg "gaia/plugins/shared/sanitize"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ChatPlugin struct {
	providers map[string]ask.Provider
}

func NewChatPlugin() *ChatPlugin {
	p := &ChatPlugin{
		providers: map[string]ask.Provider{},
	}
	p.RegisterProvider(ask.NewOllamaProvider())
	p.RegisterProvider(ask.NewOpenAIProvider())
	p.RegisterProvider(ask.NewMistralProvider())
	return p
}

func (p *ChatPlugin) ID() string           { return "chat" }
func (p *ChatPlugin) DefaultEnabled() bool { return false }
func (p *ChatPlugin) DependsOn() []string  { return nil }
func (p *ChatPlugin) ConfigSchema() []string {
	return []string{
		"chat.provider",
		"chat.host",
		"chat.port",
		"chat.model",
		"chat.timeout_seconds",
		"chat.role",
	}
}

func (p *ChatPlugin) RegisterProvider(provider ask.Provider) {
	if provider == nil {
		return
	}
	p.providers[provider.Name()] = provider
}

func (p *ChatPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start a chat session",
		RunE: func(cmd *cobra.Command, args []string) error {
			req := ask.AskRequest{
				Provider:        ask.FirstNonEmpty(viper.GetString("chat.provider"), viper.GetString("provider")),
				Host:            ask.FirstNonEmpty(viper.GetString("chat.host"), viper.GetString("host")),
				Port:            ask.FirstNonZero(viper.GetInt("chat.port"), viper.GetInt("port")),
				Model:           ask.FirstNonEmpty(viper.GetString("chat.model"), viper.GetString("model")),
				Timeout:         time.Duration(ask.FirstNonZero(viper.GetInt("chat.timeout_seconds"), viper.GetInt("timeout_seconds"))) * time.Second,
				SystemPrompt:    "",
				Pull:            false,
				ProgressOut:     cmd.ErrOrStderr(),
				ProgressClearer: &shared.ProgressClearer{},
			}
			if req.Timeout == 0 {
				req.Timeout = 120 * time.Second
			}
			if strings.TrimSpace(req.Provider) == "" {
				req.Provider = ask.ResolveProviderFromModel(req.Model)
			}
			if err := validateChatConfig(req); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}

			provider, ok := p.providers[req.Provider]
			if !ok {
				if fallback, hasFallback := p.providers["ollama"]; hasFallback {
					provider = fallback
				} else {
					return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Unknown provider %q", req.Provider))
				}
			}

			_ = shared.PrintBox(cmd.OutOrStdout(), "Chat", "Starting chat session. Type 'exit' to end.")
			reader := bufio.NewReader(cmd.InOrStdin())
			history := []ask.ChatMessage{}
			sessionID := time.Now().UTC().Format("20060102T150405.000000000Z")
			assistantTurns := 0
			noCache, _ := cmd.Flags().GetBool("no-cache")
			refreshCache, _ := cmd.Flags().GetBool("refresh-cache")
			if !cmd.Flags().Lookup("refresh-cache").Changed {
				refreshCache = viper.GetBool("cache.refresh")
			}
			if noCache {
				refreshCache = false
			}
			canRead := cache.Enabled() && !noCache && !refreshCache
			canWrite := cache.Enabled() && !noCache
			baseRole := strings.TrimSpace(viper.GetString("chat.role"))
			if pull, _ := cmd.Flags().GetBool("pull"); pull {
				req.Pull = true
			}

			for {
				_ = shared.PrintPrompt(cmd.OutOrStdout(), "You: ")
				line, err := reader.ReadString('\n')
				if err != nil {
					if err == io.EOF {
						_ = shared.PrintBox(cmd.OutOrStdout(), "Chat", "Chat session ended (EOF).")
						return nil
					}
					return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Error reading input: %v", err))
				}
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if strings.EqualFold(line, "exit") {
					_ = shared.PrintBox(cmd.OutOrStdout(), "Chat", "Chat session ended.")
					return nil
				}

				history = append(history, ask.ChatMessage{Role: "user", Content: line})
				req.Messages = history
				req.SystemPrompt = ""
				roleName := baseRole
				if roleName == "" && viper.GetBool("roles.auto_select") {
					kw := loadRoleKeywords()
					weight := viper.GetFloat64("roles.scoring.weight")
					if weight == 0 {
						weight = 1.0
					}
					threshold := viper.GetFloat64("roles.scoring.min_threshold")
					defaultRole := viper.GetString("roles.default_role")
					res := roles.SelectRoleForText(line, kw, weight, threshold, defaultRole)
					roleName = res.RoleName
					if viper.GetBool("roles.debug") {
						roles.SetDebugWriter(cmd.ErrOrStderr())
						roles.LogScores(res.AllScores, res.Threshold, res.RoleName)
					}
				}
				if roleName != "" {
					rolesList, err := loadRolesFromConfig()
					if err != nil {
						_ = shared.PrintError(cmd.ErrOrStderr(), err.Error())
						continue
					}
					resolved, err := roles.ResolveInheritance(rolesList)
					if err != nil {
						_ = shared.PrintError(cmd.ErrOrStderr(), err.Error())
						continue
					}
					role, ok := resolved[roleName]
					if !ok {
						_ = shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("role %q not found", roleName))
						continue
					}
					req.SystemPrompt = roles.ResolveSystemPrompt(role, req.Provider, req.Model)
				}
				if memCtx, err := mempalace.InjectIfEnabled(cmd.Context(), line); err != nil {
					_ = shared.PrintError(cmd.ErrOrStderr(), err.Error())
					continue
				} else if memCtx != "" {
					req.SystemPrompt = mempalace.AppendMemory(req.SystemPrompt, memCtx)
				}

				cacheKey := ""
				if canWrite {
					label := ask.BuildLabel("chat", line)
					keyPayload := cache.KeyPayload{
						PluginID: "chat",
						Provider: provider.Name(),
						Host:     req.Host,
						Port:     req.Port,
						Model:    req.Model,
						Messages: toCacheMessages(history),
						Label:    label,
					}
					key, err := cache.BuildKey(keyPayload)
					if err == nil {
						cacheKey = key
						if canRead {
							if entry, ok, err := cache.Get(cacheKey); err == nil && ok {
								history = append(history, ask.ChatMessage{Role: "assistant", Content: entry.Response})
								assistantTurns++
								if err := mempalace.PersistChatTurn(cmd.Context(), sessionID, assistantTurns, line, entry.Response); err != nil {
									return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("mempalace add drawer failed: %v", err))
								}
								_ = shared.PrintBox(cmd.OutOrStdout(), "Assistant", entry.Response)
								continue
							}
						}
					}
				}

				sreq := applySanitize(cmd, req)
				finalText, err := shared.DisplayStreamedAnswer(cmd.Context(), cmd.OutOrStdout(), "Assistant", func(send func(string)) (string, error) {
					var streamed strings.Builder
					cleared := false
					resp, streamErr := provider.SendStream(cmd.Context(), sreq, func(chunk string) {
						if strings.TrimSpace(chunk) == "" {
							return
						}
						if !cleared {
							sreq.ProgressClearer.ClearOnce(cmd.ErrOrStderr())
							cleared = true
						}
						send(chunk)
						streamed.WriteString(chunk)
					})
					if streamErr != nil {
						return "", streamErr
					}
					if resp.Text == "" {
						resp.Text = streamed.String()
					}
					return resp.Text, nil
				})
				if err != nil {
					_ = shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Ask failed: %v", err))
					continue
				}
				if strings.TrimSpace(finalText) == "" {
					_ = shared.PrintError(cmd.ErrOrStderr(), "Ask returned an empty response")
					continue
				}
				history = append(history, ask.ChatMessage{Role: "assistant", Content: finalText})
				assistantTurns++
				if err := mempalace.PersistChatTurn(cmd.Context(), sessionID, assistantTurns, line, finalText); err != nil {
					return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("mempalace add drawer failed: %v", err))
				}
				if err := mempalace.DiaryWriteIfEnabled(cmd.Context(), line, finalText); err != nil && viper.GetBool("debug") {
					_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] mempalace diary write failed: %v\n", err))
				}
				if canWrite && cacheKey != "" {
					_ = cache.Set(cache.Entry{
						Key:       cacheKey,
						Label:     ask.BuildLabel("chat", line),
						PluginID:  "chat",
						Provider:  provider.Name(),
						Host:      req.Host,
						Port:      req.Port,
						Model:     req.Model,
						Messages:  toCacheMessages(history),
						Response:  finalText,
						CreatedAt: time.Now().UTC(),
					})
				}
			}
		},
	}

	cmd.Flags().String("host", "", "Provider host (overrides chat.host)")
	cmd.Flags().Int("port", 0, "Provider port (overrides chat.port)")
	cmd.Flags().String("model", "", "Model name (overrides chat.model)")
	cmd.Flags().Int("timeout", 0, "Request timeout in seconds (overrides chat.timeout_seconds)")
	cmd.Flags().Bool("no-cache", false, "Disable cache for this session")
	cmd.Flags().Bool("refresh-cache", false, "Refresh cache for this session")
	cmd.Flags().String("role", "", "Role name to apply to the session")
	cmd.Flags().Bool("pull", false, "Pull model from Ollama if available (force refresh)")

	_ = viper.BindPFlag("chat.host", cmd.Flags().Lookup("host"))
	_ = viper.BindPFlag("chat.port", cmd.Flags().Lookup("port"))
	_ = viper.BindPFlag("chat.model", cmd.Flags().Lookup("model"))
	_ = viper.BindPFlag("chat.timeout_seconds", cmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("cache.refresh", cmd.Flags().Lookup("refresh-cache"))
	_ = viper.BindPFlag("chat.role", cmd.Flags().Lookup("role"))

	return []*cobra.Command{cmd}, nil
}

func validateChatConfig(req ask.AskRequest) error {
	missing := []string{}
	if strings.TrimSpace(req.Provider) == "" {
		missing = append(missing, "model")
	}
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
		return fmt.Errorf("missing chat configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}

func toCacheMessages(history []ask.ChatMessage) []cache.Message {
	out := make([]cache.Message, 0, len(history))
	for _, msg := range history {
		if strings.TrimSpace(msg.Role) == "" || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		out = append(out, cache.Message{
			Role:    msg.Role,
			Content: msg.Content,
		})
	}
	return out
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

func applySanitize(cmd *cobra.Command, req ask.AskRequest) ask.AskRequest {
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
			_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] sanitize: %v\n", err))
		}
		return req
	}
	if opts.LogStats && (stats.TokensBefore > 0 || stats.TokensAfter > 0) {
		_ = shared.PrintRaw(cmd.ErrOrStderr(),
			fmt.Sprintf("[sanitize] tokens before=%d after=%d removed≈%d ms=%d\n",
				stats.TokensBefore, stats.TokensAfter, stats.RemovedCount, stats.DurationMillis))
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
