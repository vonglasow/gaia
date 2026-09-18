// Package chat holds a conversation with a model, one turn at a time.
package chat

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"gaia/kernel"
	"gaia/plugins/ask"
	"gaia/plugins/mempalace"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type ChatPlugin struct {
	kernel.BasePlugin

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
func (p *ChatPlugin) DefaultEnabled() bool { return true }
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

func (p *ChatPlugin) Register(_ *kernel.Kernel) ([]*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "chat",
		Short: "Start a chat session",
		RunE: func(cmd *cobra.Command, _ []string) error {
			model, _ := cmd.Flags().GetString("model")
			req, err := ask.Endpoint{
				Plugin: "chat", Model: model, Provider: providerFlag(cmd),
			}.Resolve()
			if err != nil {
				return shared.Fail(cmd.ErrOrStderr(), err.Error())
			}
			req.ProgressOut = cmd.ErrOrStderr()
			req.ProgressClearer = &shared.ProgressClearer{}
			provider, err := ask.ProviderFor(p.providers, req.Provider)
			if err != nil {
				return shared.Fail(cmd.ErrOrStderr(), err.Error())
			}

			_ = shared.PrintBox(cmd.OutOrStdout(), "Chat", "Starting chat session. Type 'exit' to end.")
			reader := bufio.NewReader(cmd.InOrStdin())
			sessionID := time.Now().UTC().Format("20060102T150405.000000000Z")
			assistantTurns := 0
			exchange := ask.NewExchange(cmd, "chat")
			state := &session{role: strings.TrimSpace(viper.GetString("chat.role"))}
			if pull, _ := cmd.Flags().GetBool("pull"); pull {
				req.Pull = true
			}

			for {
				_ = shared.PrintPrompt(cmd.OutOrStdout(), "You: ")
				line, err := reader.ReadString('\n')
				if err != nil {
					if errors.Is(err, io.EOF) {
						_ = shared.PrintBox(cmd.OutOrStdout(), "Chat", "Chat session ended (EOF).")
						return nil
					}
					return shared.Fail(cmd.ErrOrStderr(), fmt.Sprintf("Error reading input: %v", err))
				}
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				if done := state.run(line); done.handled {
					_ = shared.PrintBox(cmd.OutOrStdout(), "Chat", done.reply)
					if done.quit {
						return nil
					}
					continue
				}

				state.history = append(state.history, ask.ChatMessage{Role: "user", Content: line})
				req.Messages = state.history
				req.SystemPrompt = ""
				if ctxPrompt, err := mempalace.SearchContextIfEnabled(cmd.Context(), line); err != nil {
					_ = shared.PrintError(cmd.ErrOrStderr(), err.Error())
					continue
				} else if ctxPrompt != "" {
					req.SystemPrompt = ctxPrompt
				} else {
					prompt, err := ask.RolePromptFor(cmd, state.role, line, req)
					if err != nil {
						_ = shared.PrintError(cmd.ErrOrStderr(), err.Error())
						continue
					}
					req.SystemPrompt = prompt
				}
				if memCtx, err := mempalace.InjectIfEnabled(cmd.Context(), line); err != nil {
					_ = shared.PrintError(cmd.ErrOrStderr(), err.Error())
					continue
				} else if memCtx != "" {
					req.SystemPrompt = mempalace.AppendMemory(req.SystemPrompt, memCtx)
				}

				if answered, ok := exchange.Lookup(line, req, provider.Name(), ask.ToCacheMessages(state.history)); ok {
					state.history = append(state.history, ask.ChatMessage{Role: "assistant", Content: answered})
					assistantTurns++
					rememberTurn(cmd, sessionID, assistantTurns, line, answered)
					_ = shared.PrintBox(cmd.OutOrStdout(), "Assistant", answered)
					continue
				}

				finalText, err := ask.StreamAnswer(cmd, "Assistant", provider, ask.ApplySanitize(cmd.ErrOrStderr(), req))
				if err != nil {
					_ = shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Ask failed: %v", err))
					continue
				}
				if strings.TrimSpace(finalText) == "" {
					_ = shared.PrintError(cmd.ErrOrStderr(), "Ask returned an empty response")
					continue
				}
				state.history = append(state.history, ask.ChatMessage{Role: "assistant", Content: finalText})
				assistantTurns++
				rememberTurn(cmd, sessionID, assistantTurns, line, finalText)
				exchange.Store(finalText)
			}
		},
	}

	ask.AddEndpointFlags(cmd, "chat", "session")

	return []*cobra.Command{cmd}, nil
}

// providerFlag prefers what was typed, so one session can go elsewhere.
func providerFlag(cmd *cobra.Command) string {
	value, err := cmd.Flags().GetString("provider")
	if err != nil {
		return ""
	}
	return value
}

// rememberTurn records one exchange, and says nothing unless asked to.
func rememberTurn(cmd *cobra.Command, sessionID string, turn int, question, answer string) {
	if err := mempalace.PersistChatTurn(cmd.Context(), sessionID, turn, question, answer); err != nil && viper.GetBool("debug") {
		_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] mempalace persist failed: %v\n", err))
	}
	if err := mempalace.DiaryWriteIfEnabled(cmd.Context(), question, answer); err != nil && viper.GetBool("debug") {
		_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] mempalace diary write failed: %v\n", err))
	}
}
