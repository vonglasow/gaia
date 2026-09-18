package ask

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gaia/kernel"
	"gaia/plugins/cache"
	"gaia/plugins/mempalace"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type AskPlugin struct {
	kernel.BasePlugin

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

func (p *AskPlugin) Register(_ *kernel.Kernel) ([]*cobra.Command, error) {
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
				return shared.Fail(cmd.ErrOrStderr(), "No message provided. Pass text or pipe input.")
			}

			typedProvider, _ := cmd.Flags().GetString("provider")
			req, err := Endpoint{Plugin: "ask", Provider: typedProvider}.Resolve()
			if err != nil {
				return shared.Fail(cmd.ErrOrStderr(), err.Error())
			}
			req.Message = msg
			req.ProgressOut = cmd.ErrOrStderr()
			req.ProgressClearer = &shared.ProgressClearer{}
			req.Pull, _ = cmd.Flags().GetBool("pull")
			if err := resolveSystemPrompt(cmd, &req, msg); err != nil {
				return shared.Fail(cmd.ErrOrStderr(), err.Error())
			}
			if memCtx, err := mempalace.InjectIfEnabled(cmd.Context(), msg); err != nil {
				return shared.Fail(cmd.ErrOrStderr(), err.Error())
			} else if memCtx != "" {
				req.SystemPrompt = mempalace.AppendMemory(req.SystemPrompt, memCtx)
			}

			provider, ok := p.providers[req.Provider]
			if !ok {
				if fallback, hasFallback := p.providers["ollama"]; hasFallback {
					provider = fallback
				} else {
					return shared.Fail(cmd.ErrOrStderr(), fmt.Sprintf("Unknown provider %q", req.Provider))
				}
			}

			exchange := NewExchange(cmd, "ask")
			asked := []cache.Message{{Role: "user", Content: msg}}
			if answered, ok := exchange.Lookup(msg, req, provider.Name(), asked); ok {
				return shared.PrintBox(cmd.OutOrStdout(), "Answer", answered)
			}

			finalText, err := StreamAnswer(cmd, "Answer", provider, ApplySanitize(cmd.ErrOrStderr(), req))
			if err != nil {
				return shared.Fail(cmd.ErrOrStderr(), fmt.Sprintf("Ask failed: %v", err))
			}
			if strings.TrimSpace(finalText) == "" {
				return shared.Fail(cmd.ErrOrStderr(), "Ask returned an empty response")
			}
			exchange.Store(finalText)
			if err := mempalace.PersistAskResponse(cmd.Context(), msg, finalText); err != nil && viper.GetBool("debug") {
				_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] mempalace persist failed: %v\n", err))
			}
			if err := mempalace.DiaryWriteIfEnabled(cmd.Context(), msg, finalText); err != nil && viper.GetBool("debug") {
				_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] mempalace diary write failed: %v\n", err))
			}
			return nil
		},
	}

	AddEndpointFlags(cmd, "ask", "request")

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
	// Tools offered to the model; empty stays a plain question.
	Tools []ToolSpec
	// KeepAlive is how long Ollama holds the model after answering; "0" drops it at once.
	KeepAlive string
	// ContextWindow fixes num_ctx for a whole run. Changing it between turns
	// makes Ollama reload the model, which costs seconds every time.
	ContextWindow int
}

type AskResponse struct {
	Text string
	// ToolCalls is what the model asked to run, alongside or instead of text.
	ToolCalls []ToolCall
}

type ChatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
	// ToolCalls replay what an assistant turn asked for.
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	// ToolName pairs a result with its call, which several in flight need.
	ToolName string `json:"tool_name,omitempty"`
}

type Provider interface {
	Name() string
	Send(ctx context.Context, req AskRequest) (AskResponse, error)
	SendStream(ctx context.Context, req AskRequest, onChunk func(string)) (AskResponse, error)
}

func ResolveProviderFromModel(model string) string {
	name := strings.ToLower(strings.TrimSpace(model))
	if name == "" {
		return ""
	}
	// A tag is Ollama's way of naming a pull; no hosted API uses one.
	if strings.Contains(name, ":") {
		return "ollama"
	}
	if strings.HasPrefix(name, "gpt-") || strings.HasPrefix(name, "o3-") || strings.HasPrefix(name, "o4-") {
		return "openai"
	}
	// Hosted Mistral models are hyphenated — mistral-large, open-mistral-7b.
	// A bare "mistral" is what `ollama pull mistral` leaves behind.
	if strings.HasPrefix(name, "mistral-") || strings.HasPrefix(name, "open-mistral") ||
		strings.HasPrefix(name, "open-mixtral") || strings.HasPrefix(name, "ministral-") {
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

// KeepAlive is how long Ollama holds a model after answering; its default is five minutes.
func KeepAlive(req AskRequest) string {
	if value := strings.TrimSpace(req.KeepAlive); value != "" {
		return value
	}
	return strings.TrimSpace(viper.GetString("ollama.keep_alive"))
}
