package ask

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"gaia/kernel"
	"gaia/plugins/cache"
	"gaia/plugins/mempalace"
	"gaia/plugins/roles"
	"gaia/plugins/shared"
	sanitizepkg "gaia/plugins/shared/sanitize"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type AskPlugin struct {
	providers map[string]Provider
}

func NewAskPlugin() *AskPlugin {
	p := &AskPlugin{
		providers: map[string]Provider{},
	}
	p.RegisterProvider(NewOllamaProvider())
	p.RegisterProvider(NewOpenAIProvider())
	p.RegisterProvider(NewMistralProvider())
	return p
}

func (p *AskPlugin) ID() string           { return "ask" }
func (p *AskPlugin) DefaultEnabled() bool { return true }
func (p *AskPlugin) DependsOn() []string  { return nil }
func (p *AskPlugin) ConfigSchema() []string {
	return []string{
		"ask.provider",
		"ask.host",
		"ask.port",
		"ask.model",
		"ask.timeout_seconds",
		"ask.role",
	}
}

func (p *AskPlugin) RegisterProvider(provider Provider) {
	if provider == nil {
		return
	}
	p.providers[provider.Name()] = provider
}

func (p *AskPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "ask [message]",
		Short: "Ask a model (via provider)",
		Args:  cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			msg := strings.TrimSpace(strings.Join(args, " "))
			if msg == "" {
				msg = strings.TrimSpace(readStdin(cmd.InOrStdin()))
			}
			if msg == "" {
				return shared.PrintError(cmd.ErrOrStderr(), "No message provided. Pass text or pipe input.")
			}

			req := AskRequest{
				Provider:        FirstNonEmpty(viper.GetString("ask.provider"), viper.GetString("provider")),
				Host:            FirstNonEmpty(viper.GetString("ask.host"), viper.GetString("host")),
				Port:            FirstNonZero(viper.GetInt("ask.port"), viper.GetInt("port")),
				Model:           FirstNonEmpty(viper.GetString("ask.model"), viper.GetString("model")),
				Timeout:         time.Duration(FirstNonZero(viper.GetInt("ask.timeout_seconds"), viper.GetInt("timeout_seconds"))) * time.Second,
				SystemPrompt:    "",
				Message:         msg,
				Pull:            false,
				ProgressOut:     cmd.ErrOrStderr(),
				ProgressClearer: &shared.ProgressClearer{},
			}
			if req.Timeout == 0 {
				req.Timeout = 120 * time.Second
			}
			if strings.TrimSpace(req.Provider) == "" {
				req.Provider = ResolveProviderFromModel(req.Model)
			}
			if pull, _ := cmd.Flags().GetBool("pull"); pull {
				req.Pull = true
			}
			if err := resolveSystemPrompt(cmd, &req, msg); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			if err := validateAskConfig(req); err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			if memCtx, err := mempalace.InjectIfEnabled(cmd.Context(), msg); err != nil {
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

			noCache, _ := cmd.Flags().GetBool("no-cache")
			refreshCache, _ := cmd.Flags().GetBool("refresh-cache")
			if !cmd.Flags().Lookup("refresh-cache").Changed {
				refreshCache = viper.GetBool("cache.refresh")
			}
			if noCache {
				refreshCache = false
			}
			cacheKey := ""
			canRead := cache.Enabled() && !noCache && !refreshCache
			canWrite := cache.Enabled() && !noCache
			if canWrite {
				label := BuildLabel("ask", msg)
				keyPayload := cache.KeyPayload{
					PluginID: "ask",
					Provider: provider.Name(),
					Host:     req.Host,
					Port:     req.Port,
					Model:    req.Model,
					Messages: []cache.Message{{Role: "user", Content: msg}},
					Label:    label,
				}
				key, err := cache.BuildKey(keyPayload)
				if err == nil {
					cacheKey = key
					if canRead {
						if entry, ok, err := cache.Get(cacheKey); err == nil && ok {
							return shared.PrintBox(cmd.OutOrStdout(), "Answer", entry.Response)
						}
					}
				}
			}

			sreq := ApplySanitize(cmd.ErrOrStderr(), req)
			finalText, err := shared.DisplayStreamedAnswer(cmd.Context(), cmd.OutOrStdout(), "Answer", func(send func(string)) (string, error) {
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
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Ask failed: %v", err))
			}
			if strings.TrimSpace(finalText) == "" {
				return shared.PrintError(cmd.ErrOrStderr(), "Ask returned an empty response")
			}
			if canWrite && cacheKey != "" {
				_ = cache.Set(cache.Entry{
					Key:       cacheKey,
					Label:     BuildLabel("ask", msg),
					PluginID:  "ask",
					Provider:  provider.Name(),
					Host:      req.Host,
					Port:      req.Port,
					Model:     req.Model,
					Messages:  []cache.Message{{Role: "user", Content: msg}},
					Response:  finalText,
					CreatedAt: time.Now().UTC(),
				})
			}
			if err := mempalace.PersistAskResponse(cmd.Context(), msg, finalText); err != nil && viper.GetBool("debug") {
				_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] mempalace persist failed: %v\n", err))
			}
			if err := mempalace.DiaryWriteIfEnabled(cmd.Context(), msg, finalText); err != nil && viper.GetBool("debug") {
				_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] mempalace diary write failed: %v\n", err))
			}
			return nil
		},
	}

	cmd.Flags().String("provider", "", "Provider name (overrides ask.provider)")
	cmd.Flags().String("host", "", "Provider host (overrides ask.host)")
	cmd.Flags().Int("port", 0, "Provider port (overrides ask.port)")
	cmd.Flags().String("model", "", "Model name (overrides ask.model)")
	cmd.Flags().Int("timeout", 0, "Request timeout in seconds (overrides ask.timeout_seconds)")
	cmd.Flags().Bool("no-cache", false, "Disable cache for this request")
	cmd.Flags().Bool("refresh-cache", false, "Refresh cache for this request")
	cmd.Flags().String("role", "", "Role name to apply to the request")
	cmd.Flags().Bool("pull", false, "Pull model from Ollama if available (force refresh)")

	_ = viper.BindPFlag("ask.host", cmd.Flags().Lookup("host"))
	_ = viper.BindPFlag("ask.port", cmd.Flags().Lookup("port"))
	_ = viper.BindPFlag("ask.model", cmd.Flags().Lookup("model"))
	_ = viper.BindPFlag("ask.timeout_seconds", cmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag("cache.refresh", cmd.Flags().Lookup("refresh-cache"))
	_ = viper.BindPFlag("ask.role", cmd.Flags().Lookup("role"))

	return []*cobra.Command{cmd}, nil
}

type AskRequest struct {
	Provider        string
	Host            string
	Port            int
	Model           string
	Timeout         time.Duration
	SystemPrompt    string
	Message         string
	Messages        []ChatMessage
	Pull            bool
	ProgressOut     io.Writer
	ProgressClearer *shared.ProgressClearer
}

type AskResponse struct {
	Text string
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type Provider interface {
	Name() string
	Send(ctx context.Context, req AskRequest) (AskResponse, error)
	SendStream(ctx context.Context, req AskRequest, onChunk func(string)) (AskResponse, error)
}

type OllamaProvider struct{}

func NewOllamaProvider() *OllamaProvider { return &OllamaProvider{} }

func (p *OllamaProvider) Name() string { return "ollama" }

func (p *OllamaProvider) Send(ctx context.Context, req AskRequest) (AskResponse, error) {
	if err := p.ensureModel(ctx, req); err != nil {
		return AskResponse{}, err
	}
	reqCtx, cancel := withTimeout(ctx, req.Timeout)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d/api/chat", req.Host, req.Port)
	messages := buildMessages(req)
	payload := map[string]any{
		"model":    req.Model,
		"stream":   false,
		"messages": messages,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return AskResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return AskResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: req.Timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return AskResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return AskResponse{}, fmt.Errorf("ollama error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var decoded struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return AskResponse{}, err
	}
	return AskResponse{Text: decoded.Message.Content}, nil
}

func (p *OllamaProvider) SendStream(ctx context.Context, req AskRequest, onChunk func(string)) (AskResponse, error) {
	if err := p.ensureModel(ctx, req); err != nil {
		return AskResponse{}, err
	}
	reqCtx, cancel := withTimeout(ctx, req.Timeout)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d/api/chat", req.Host, req.Port)
	payload := map[string]any{
		"model":    req.Model,
		"stream":   true,
		"messages": buildMessages(req),
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return AskResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return AskResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: req.Timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return AskResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return AskResponse{}, fmt.Errorf("ollama error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var full strings.Builder
	decoder := json.NewDecoder(resp.Body)
	for {
		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done bool `json:"done"`
		}
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return AskResponse{}, err
		}
		if chunk.Message.Content != "" {
			onChunk(chunk.Message.Content)
			full.WriteString(chunk.Message.Content)
		}
		if chunk.Done {
			break
		}
	}
	return AskResponse{Text: full.String()}, nil
}

func (p *OllamaProvider) ensureModel(ctx context.Context, req AskRequest) error {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil
	}
	client := &http.Client{Timeout: req.Timeout}
	baseURL := fmt.Sprintf("http://%s:%d", req.Host, req.Port)
	exists, err := p.modelExists(ctx, client, baseURL, model)
	if err != nil {
		return err
	}
	if exists && !req.Pull {
		return nil
	}
	return p.pullModel(ctx, client, baseURL, model, req.ProgressOut, req.ProgressClearer)
}

func (p *OllamaProvider) modelExists(ctx context.Context, client *http.Client, baseURL, model string) (bool, error) {
	url := fmt.Sprintf("%s/api/tags", baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return false, fmt.Errorf("ollama tags error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var decoded struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return false, err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return false, nil
	}
	hasTag := strings.Contains(model, ":")
	for _, entry := range decoded.Models {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		if hasTag {
			if name == model {
				return true, nil
			}
			continue
		}
		if name == model || strings.HasPrefix(name, model+":") {
			return true, nil
		}
	}
	return false, nil
}

func (p *OllamaProvider) pullModel(_ context.Context, client *http.Client, baseURL, model string, out io.Writer, clearer *shared.ProgressClearer) error {
	pullCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	url := fmt.Sprintf("%s/api/pull", baseURL)
	payload := map[string]any{
		"name":   model,
		"stream": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(pullCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("ollama pull error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	type pullEvent struct {
		Status    string `json:"status"`
		Completed int64  `json:"completed"`
		Total     int64  `json:"total"`
		Error     string `json:"error"`
		Done      bool   `json:"done"`
	}
	if out == nil {
		out = io.Discard
	}

	progressModel := shared.NewProgressModel(50)
	cancelled := false
	var closeOnce sync.Once
	progressModel.OnCancel = func() {
		cancelled = true
		cancel()
		closeOnce.Do(func() {
			_ = resp.Body.Close()
		})
	}
	prg := shared.NewProgressProgram(progressModel, out)
	errCh := make(chan error, 1)

	go func() {
		defer func() {
			prg.Send(shared.ProgressDone{})
			close(errCh)
		}()
		decoder := json.NewDecoder(resp.Body)
		for {
			var event pullEvent
			if err := decoder.Decode(&event); err != nil {
				if err == io.EOF {
					return
				}
				if pullCtx.Err() != nil {
					return
				}
				errCh <- err
				return
			}
			if strings.TrimSpace(event.Error) != "" {
				errCh <- fmt.Errorf("ollama pull error: %s", strings.TrimSpace(event.Error))
				return
			}
			if event.Completed > 0 || event.Total > 0 {
				prg.Send(shared.ProgressUpdate{Completed: event.Completed, Total: event.Total})
			}
			if event.Done {
				return
			}
		}
	}()

	if _, err := prg.Run(); err != nil {
		return fmt.Errorf("error running progress UI: %v", err)
	}
	if cancelled {
		return fmt.Errorf("ollama pull canceled")
	}
	if clearer != nil {
		clearer.MarkPending()
	}
	if err, ok := <-errCh; ok && err != nil {
		return err
	}
	return nil
}

func validateAskConfig(req AskRequest) error {
	missing := []string{}
	if strings.TrimSpace(req.Provider) == "" {
		missing = append(missing, "ask.provider")
	}
	if strings.TrimSpace(req.Host) == "" {
		missing = append(missing, "ask.host")
	}
	if req.Port == 0 {
		missing = append(missing, "ask.port")
	}
	if strings.TrimSpace(req.Model) == "" {
		missing = append(missing, "ask.model")
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing ask configuration: %s", strings.Join(missing, ", "))
	}
	return nil
}

func ResolveProviderFromModel(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return ""
	}
	if strings.HasPrefix(name, "gpt-") || strings.HasPrefix(name, "o3-") || strings.HasPrefix(name, "o4-") {
		return "openai"
	}
	if strings.HasPrefix(name, "mistral") {
		return "mistral"
	}
	return "ollama"
}

func BuildLabel(pluginID, message string) string {
	label := strings.TrimSpace(message)
	if label == "" {
		return pluginID
	}
	if len(label) > 120 {
		label = label[:120] + "..."
	}
	return label
}

func buildMessages(req AskRequest) []map[string]string {
	out := []map[string]string{}
	if strings.TrimSpace(req.SystemPrompt) != "" {
		out = append(out, map[string]string{"role": "system", "content": req.SystemPrompt})
	}
	if len(req.Messages) > 0 {
		for _, msg := range req.Messages {
			if strings.TrimSpace(msg.Role) == "" || strings.TrimSpace(msg.Content) == "" {
				continue
			}
			out = append(out, map[string]string{"role": msg.Role, "content": msg.Content})
		}
		return out
	}
	if strings.TrimSpace(req.Message) != "" {
		out = append(out, map[string]string{"role": "user", "content": req.Message})
	}
	return out
}

func FirstNonEmpty(primary, fallback string) string {
	if strings.TrimSpace(primary) != "" {
		return primary
	}
	return fallback
}

func FirstNonZero(primary, fallback int) int {
	if primary != 0 {
		return primary
	}
	return fallback
}

func readStdin(r io.Reader) string {
	info, err := os.Stdin.Stat()
	if err == nil && (info.Mode()&os.ModeCharDevice) == 0 {
		reader := bufio.NewReader(r)
		b, _ := io.ReadAll(reader)
		return string(b)
	}
	return ""
}

func withTimeout(ctx context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	if timeout <= 0 {
		return ctx, func() {}
	}
	return context.WithTimeout(ctx, timeout)
}

// ApplySanitize sanitizes the conversation messages in req based on sanitize config.
// errOut receives debug log lines; pass cmd.ErrOrStderr() from callers.
func ApplySanitize(errOut io.Writer, req AskRequest) AskRequest {
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
	out, stats, err := sanitizepkg.Sanitize(sanitizepkg.Request{Messages: raw}, opts)
	if err != nil {
		if viper.GetBool("debug") {
			_ = shared.PrintRaw(errOut, fmt.Sprintf("[DEBUG] sanitize: %v\n", err))
		}
		return req
	}
	if opts.LogStats && (stats.TokensBefore > 0 || stats.TokensAfter > 0) {
		_ = shared.PrintRaw(errOut,
			fmt.Sprintf("[sanitize] tokens before=%d after=%d removed≈%d ms=%d\n",
				stats.TokensBefore, stats.TokensAfter, stats.RemovedCount, stats.DurationMillis))
	}
	req.SystemPrompt = ""
	req.Message = ""
	req.Messages = make([]ChatMessage, 0, len(out.Messages))
	for _, m := range out.Messages {
		req.Messages = append(req.Messages, ChatMessage{Role: m.Role, Content: m.Content})
	}
	return req
}

func buildMessagesForSanitize(req AskRequest) []sanitizepkg.Message {
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

// resolveSystemPrompt sets req.SystemPrompt from MemPalace context when enabled,
// falling back to role-based selection when MemPalace is unavailable or returns nothing.
func resolveSystemPrompt(cmd *cobra.Command, req *AskRequest, msg string) error {
	ctxPrompt, err := mempalace.SearchContextIfEnabled(cmd.Context(), msg)
	if err != nil {
		return err
	}
	if ctxPrompt != "" {
		req.SystemPrompt = ctxPrompt
		return nil
	}
	return applyRolePrompt(cmd, req, msg)
}

func applyRolePrompt(cmd *cobra.Command, req *AskRequest, msg string) error {
	roleName := strings.TrimSpace(viper.GetString("ask.role"))
	if roleName == "" && viper.GetBool("roles.auto_select") {
		kw := roles.LoadKeywordConfig()
		weight := viper.GetFloat64("roles.scoring.weight")
		if weight == 0 {
			weight = 1.0
		}
		threshold := viper.GetFloat64("roles.scoring.min_threshold")
		defaultRole := viper.GetString("roles.default_role")
		res := roles.SelectRoleForText(msg, kw, weight, threshold, defaultRole)
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
