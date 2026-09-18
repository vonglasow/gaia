package ask

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gaia/plugins/cache"
	"gaia/plugins/roles"
	"gaia/plugins/shared"
)

// One exchange with a model: which role it speaks under, whether the answer was
// already given, and how it is shown as it arrives. `gaia ask` and `gaia chat`
// both do all three, and used to do them in their own words.

// RolePromptFor resolves the role in force, picking one from the text when
// auto-selection is on and nobody named one. "" means no role applies.
func RolePromptFor(cmd *cobra.Command, roleName, text string, req AskRequest) (string, error) {
	roleName = strings.TrimSpace(roleName)
	// A name that was typed is held to; one that was guessed is only a suggestion.
	asked := roleName != ""
	if !asked && viper.GetBool("roles.auto_select") {
		roleName = autoSelectRole(cmd, text)
	}
	if roleName == "" {
		return "", nil
	}
	loaded, err := roles.LoadRolesWithDefaults()
	if err != nil {
		return "", err
	}
	resolved, err := roles.ResolveInheritance(loaded)
	if err != nil {
		return "", err
	}
	role, ok := resolved[roleName]
	if !ok {
		if !asked {
			// Nobody asked for it, so the question goes without one.
			return "", nil
		}
		return "", &RoleNotFoundError{Name: roleName, Available: sortedNames(resolved)}
	}
	return roles.ResolveSystemPrompt(role, req.Provider, req.Model), nil
}

// RoleNotFoundError names a role nobody wrote, and the ones somebody did:
// the next thing anyone asks is what they should have typed.
type RoleNotFoundError struct {
	Name      string
	Available []string
}

func (e *RoleNotFoundError) Error() string {
	if len(e.Available) == 0 {
		return fmt.Sprintf("no role called %q, and there are none", e.Name)
	}
	return fmt.Sprintf("no role called %q; there is %s", e.Name, strings.Join(e.Available, ", "))
}

func sortedNames(resolved map[string]roles.ResolvedRole) []string {
	out := make([]string, 0, len(resolved))
	for name := range resolved {
		out = append(out, name)
	}
	sort.Strings(out)
	return out
}

// autoSelectRole scores the text against the configured keywords.
func autoSelectRole(cmd *cobra.Command, text string) string {
	weight := viper.GetFloat64("roles.scoring.weight")
	if weight == 0 {
		weight = 1.0
	}
	result := roles.SelectRoleForText(text, roles.LoadKeywordConfig(), weight,
		viper.GetFloat64("roles.scoring.min_threshold"), viper.GetString("roles.default_role"))
	if viper.GetBool("roles.debug") {
		roles.SetDebugWriter(cmd.ErrOrStderr())
		roles.LogScores(result.AllScores, result.Threshold, result.RoleName)
	}
	return result.RoleName
}

// Exchange is the cache for one question: whether an answer may be reused, and
// under which key this one is filed.
type Exchange struct {
	pluginID string
	read     bool
	write    bool
	key      string
	label    string
	payload  cache.KeyPayload
}

// NewExchange reads the cache flags, which every command spells the same way.
func NewExchange(cmd *cobra.Command, pluginID string) *Exchange {
	noCache, _ := cmd.Flags().GetBool("no-cache")
	refresh, _ := cmd.Flags().GetBool("refresh-cache")
	if !cmd.Flags().Lookup("refresh-cache").Changed {
		refresh = viper.GetBool("cache.refresh")
	}
	if noCache {
		refresh = false
	}
	return &Exchange{
		pluginID: pluginID,
		read:     cache.Enabled() && !noCache && !refresh,
		write:    cache.Enabled() && !noCache,
	}
}

// Lookup files this question and reports an answer already given for it.
func (e *Exchange) Lookup(question string, req AskRequest, provider string, messages []cache.Message) (string, bool) {
	e.key = ""
	if !e.write {
		return "", false
	}
	e.label = BuildLabel(e.pluginID, question)
	e.payload = cache.KeyPayload{
		PluginID: e.pluginID,
		Provider: provider,
		Host:     req.Host,
		Port:     req.Port,
		Model:    req.Model,
		Messages: messages,
		Label:    e.label,
	}
	key, err := cache.BuildKey(e.payload)
	if err != nil {
		return "", false
	}
	e.key = key
	if !e.read {
		return "", false
	}
	entry, ok, err := cache.Get(key)
	if err != nil || !ok {
		return "", false
	}
	return entry.Response, true
}

// Store files the answer under the key Lookup built, and does nothing without one.
func (e *Exchange) Store(answer string) {
	if !e.write || e.key == "" {
		return
	}
	_ = cache.Set(cache.Entry{
		Key:       e.key,
		Label:     e.label,
		PluginID:  e.pluginID,
		Provider:  e.payload.Provider,
		Host:      e.payload.Host,
		Port:      e.payload.Port,
		Model:     e.payload.Model,
		Messages:  e.payload.Messages,
		Response:  answer,
		CreatedAt: time.Now().UTC(),
	})
}

// StreamAnswer shows the answer as it arrives and returns what was said.
func StreamAnswer(cmd *cobra.Command, title string, provider Provider, req AskRequest) (string, error) {
	return shared.DisplayStreamedAnswer(cmd.Context(), cmd.OutOrStdout(), title, func(send func(string)) (string, error) {
		var streamed strings.Builder
		cleared := false
		resp, err := provider.SendStream(cmd.Context(), req, func(chunk string) {
			if strings.TrimSpace(chunk) == "" {
				return
			}
			// The spinner is cleared once, by the first chunk worth showing.
			if !cleared && req.ProgressClearer != nil {
				req.ProgressClearer.ClearOnce(cmd.ErrOrStderr())
				cleared = true
			}
			send(chunk)
			streamed.WriteString(chunk)
		})
		if err != nil {
			return "", DescribeSendError(err, req)
		}
		if resp.Text == "" {
			resp.Text = streamed.String()
		}
		return resp.Text, nil
	})
}

// ToCacheMessages drops anything half-written: a key must not depend on noise.
func ToCacheMessages(history []ChatMessage) []cache.Message {
	out := make([]cache.Message, 0, len(history))
	for _, msg := range history {
		if strings.TrimSpace(msg.Role) == "" || strings.TrimSpace(msg.Content) == "" {
			continue
		}
		out = append(out, cache.Message{Role: msg.Role, Content: msg.Content})
	}
	return out
}

// ProviderFor falls back to ollama: a name nobody registered is likelier a typo
// than a reason to refuse the run.
func ProviderFor(providers map[string]Provider, name string) (Provider, error) {
	if provider, ok := providers[name]; ok {
		return provider, nil
	}
	if fallback, ok := providers["ollama"]; ok {
		return fallback, nil
	}
	return nil, fmt.Errorf("unknown provider %q", name)
}

// AddModelFlags declares where the model is and which one it is, for any
// command that talks to one, and binds each to that plugin's own config key.
func AddModelFlags(cmd *cobra.Command, plugin string) {
	cmd.Flags().String("host", "", "Where the provider listens (overrides "+plugin+".host)")
	cmd.Flags().Int("port", 0, "Port the provider listens on (overrides "+plugin+".port)")
	cmd.Flags().String("model", "", "Model name (overrides "+plugin+".model)")
	cmd.Flags().String("provider", "", "Provider name (overrides "+plugin+".provider)")
	cmd.Flags().Int("timeout", 0, "How long one request may take, in seconds (overrides "+plugin+".timeout_seconds)")
	cmd.Flags().String("role", "", "Role to work under (overrides "+plugin+".role)")

	_ = viper.BindPFlag(plugin+".host", cmd.Flags().Lookup("host"))
	_ = viper.BindPFlag(plugin+".port", cmd.Flags().Lookup("port"))
	_ = viper.BindPFlag(plugin+".model", cmd.Flags().Lookup("model"))
	_ = viper.BindPFlag(plugin+".timeout_seconds", cmd.Flags().Lookup("timeout"))
	_ = viper.BindPFlag(plugin+".role", cmd.Flags().Lookup("role"))
}

// AddEndpointFlags is AddModelFlags plus what a cached, pullable exchange adds.
// `what` names the unit in help text: a request, a session.
func AddEndpointFlags(cmd *cobra.Command, plugin, what string) {
	AddModelFlags(cmd, plugin)
	cmd.Flags().Bool("no-cache", false, "Disable cache for this "+what)
	cmd.Flags().Bool("refresh-cache", false, "Refresh cache for this "+what)
	cmd.Flags().Bool("pull", false, "Pull model from Ollama if available (force refresh)")

	_ = viper.BindPFlag("cache.refresh", cmd.Flags().Lookup("refresh-cache"))
}

// Address is where a provider listens, with the local defaults applied. A model
// is not needed to ask Ollama what it is holding.
func Address(plugin string) (string, int) {
	host := FirstNonEmpty(viper.GetString(plugin+".host"), viper.GetString("host"))
	if strings.TrimSpace(host) == "" {
		host = "localhost"
	}
	port := FirstNonZero(viper.GetInt(plugin+".port"), viper.GetInt("port"))
	if port == 0 {
		port = 11434
	}
	return host, port
}

// DescribeSendError turns a transport failure into something to act on. A
// deadline says nothing about which deadline, or what to do about it.
func DescribeSendError(err error, req AskRequest) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || strings.Contains(err.Error(), "context deadline exceeded") {
		return fmt.Errorf("%s did not answer within %s: raise %s, or use a model that fits in memory "+
			"(`ollama ps` shows the split; anything on CPU runs at a fraction of the speed)",
			req.Model, req.Timeout, "timeout_seconds")
	}
	if strings.Contains(err.Error(), "connection refused") {
		return fmt.Errorf("nothing is listening on %s:%d: is ollama running?", req.Host, req.Port)
	}
	if strings.Contains(err.Error(), "token repeat limit") {
		return fmt.Errorf("%s got stuck repeating itself and ollama stopped it. "+
			"This is the model, not the task: a smaller one loops on long instructions "+
			"and on a large set of tools. Try a shorter task, or a stronger model",
			req.Model)
	}
	if strings.Contains(err.Error(), "no such host") || strings.Contains(err.Error(), "no route to host") {
		return fmt.Errorf("cannot reach %s:%d: check the address, and that ollama there "+
			"was started with OLLAMA_HOST=0.0.0.0 — it listens on its own loopback otherwise",
			req.Host, req.Port)
	}
	if strings.Contains(err.Error(), "i/o timeout") {
		return fmt.Errorf("no answer from %s:%d within %s: it may be reachable but not "+
			"listening, or a firewall is dropping the connection", req.Host, req.Port, req.Timeout)
	}
	return err
}
