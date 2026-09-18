package ask

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"
)

// hostAndPortOf splits a test server's address into the two fields an AskRequest.
func hostAndPortOf(t *testing.T, server *httptest.Server) (string, int) {
	t.Helper()
	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)
	return parsed.Hostname(), port
}

func TestFirstNonEmptyPrefersThePrimaryUnlessItIsBlank(t *testing.T) {
	require.Equal(t, "primary", FirstNonEmpty("primary", "fallback"))
	require.Equal(t, "fallback", FirstNonEmpty("", "fallback"))
	require.Equal(t, "fallback", FirstNonEmpty("   ", "fallback"),
		"a key set to spaces in YAML is a key nobody filled in")
}

func TestFirstNonZeroPrefersThePrimaryUnlessItIsZero(t *testing.T) {
	require.Equal(t, 8080, FirstNonZero(8080, 11434))
	require.Equal(t, 11434, FirstNonZero(0, 11434))
}

// The provider is inferred from the model name when nobody named one.
func TestResolveProviderFromModelReadsTheFamily(t *testing.T) {
	for model, want := range map[string]string{
		"gpt-4o":            "openai",
		"GPT-4O":            "openai",
		"o3-mini":           "openai",
		"o4-mini":           "openai",
		"mistral-large":     "mistral",
		"  mistral-small  ": "mistral",
		"open-mistral-7b":   "mistral",
		"ministral-3b":      "mistral",
		"llama3.1":          "ollama",
		"qwen3-coder":       "ollama",
		"":                  "",

		// `ollama pull mistral` leaves a model called mistral, and asking the
		// hosted Mistral API for it fails on a key nobody needed to have.
		"mistral":           "ollama",
		"mixtral":           "ollama",
		"mistral:latest":    "ollama",
		"mistral-large:foo": "ollama",
		"qwen3-coder:30b":   "ollama",
	} {
		require.Equalf(t, want, ResolveProviderFromModel(model), "model %q", model)
	}
}

// The label is what a person reads in `gaia cache list`.
func TestBuildLabelShortensALongMessageAndFallsBackToThePluginID(t *testing.T) {
	require.Equal(t, "ask", BuildLabel("ask", "   "))
	require.Equal(t, "a question", BuildLabel("ask", "  a question  "))

	long := make([]byte, 200)
	for i := range long {
		long[i] = 'x'
	}
	label := BuildLabel("ask", string(long))
	require.Len(t, label, 123)
	require.Contains(t, label, "...")
}

func TestBuildMessagesPutsTheSystemPromptFirst(t *testing.T) {
	msgs := buildMessages(AskRequest{SystemPrompt: "be brief", Message: "hello"})

	require.Equal(t, []map[string]string{
		{"role": "system", "content": "be brief"},
		{"role": "user", "content": "hello"},
	}, msgs)
}

// A history, when there is one, replaces the single message rather than being appended.
func TestBuildMessagesPrefersAHistoryOverASingleMessage(t *testing.T) {
	msgs := buildMessages(AskRequest{
		Message:  "ignored",
		Messages: []ChatMessage{{Role: "user", Content: "first"}, {Role: "assistant", Content: "second"}},
	})

	require.Len(t, msgs, 2)
	require.Equal(t, "first", msgs[0]["content"])
	require.Equal(t, "second", msgs[1]["content"])
}

func TestBuildMessagesDropsHalfEmptyTurns(t *testing.T) {
	msgs := buildMessages(AskRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "kept"},
			{Role: "", Content: "no role"},
			{Role: "user", Content: "   "},
		},
	})

	require.Len(t, msgs, 1)
	require.Equal(t, "kept", msgs[0]["content"])
}

func TestBuildMessagesIsEmptyWhenThereIsNothingToSay(t *testing.T) {
	require.Empty(t, buildMessages(AskRequest{}))
}

// --- the HTTP providers ---------------------------------------------------

func TestOpenAISendsAChatCompletionAndReadsTheAnswerBack(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	var seenPath, seenAuth string
	var seenBody openAIChatCompletionRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		seenAuth = r.Header.Get("Authorization")
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &seenBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"an answer"}}]}`))
	}))
	defer server.Close()

	host, port := hostAndPortOf(t, server)
	resp, err := NewOpenAIProvider().Send(context.Background(), AskRequest{
		Host: host, Port: port, Model: "gpt-4o", Message: "a question", SystemPrompt: "be brief",
	})

	require.NoError(t, err)
	require.Equal(t, "an answer", resp.Text)
	require.Equal(t, "/v1/chat/completions", seenPath)
	require.Equal(t, "Bearer test-key", seenAuth)
	require.Equal(t, "gpt-4o", seenBody.Model)
	require.False(t, seenBody.Stream, "Send is the non-streaming path")
	require.Len(t, seenBody.Messages, 2)
	require.Equal(t, "system", seenBody.Messages[0].Role)
}

// Sending without a key would reach OpenAI and come back 401.
func TestOpenAIRefusesToSendWithoutAKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	_, err := NewOpenAIProvider().Send(context.Background(), AskRequest{
		Host: "localhost", Port: 443, Model: "gpt-4o", Message: "a question",
	})

	require.ErrorContains(t, err, "OPENAI_API_KEY")
}

func TestOpenAIRefusesAnIncompleteEndpoint(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	_, err := NewOpenAIProvider().Send(context.Background(), AskRequest{Model: "gpt-4o"})

	require.ErrorContains(t, err, "host and port")
}

func TestOpenAIReportsAnErrorStatusRatherThanAnEmptyAnswer(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"upstream is down"}`))
	}))
	defer server.Close()

	host, port := hostAndPortOf(t, server)
	_, err := NewOpenAIProvider().Send(context.Background(), AskRequest{
		Host: host, Port: port, Model: "gpt-4o", Message: "a question",
	})

	require.Error(t, err)
}

func TestMistralSendsAChatCompletionAndReadsTheAnswerBack(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")

	var seenPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"an answer"}}]}`))
	}))
	defer server.Close()

	host, port := hostAndPortOf(t, server)
	resp, err := NewMistralProvider().Send(context.Background(), AskRequest{
		Host: host, Port: port, Model: "mistral-large", Message: "a question",
	})

	require.NoError(t, err)
	require.Equal(t, "an answer", resp.Text)
	require.Contains(t, seenPath, "chat/completions")
}

func TestMistralRefusesToSendWithoutAKey(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "")

	_, err := NewMistralProvider().Send(context.Background(), AskRequest{
		Host: "localhost", Port: 443, Model: "mistral-large", Message: "a question",
	})

	require.ErrorContains(t, err, "MISTRAL_API_KEY")
}

func TestEveryProviderAnswersToItsConfiguredName(t *testing.T) {
	require.Equal(t, "ollama", NewOllamaProvider().Name())
	require.Equal(t, "openai", NewOpenAIProvider().Name())
	require.Equal(t, "mistral", NewMistralProvider().Name())
}

func TestThePluginDeclaresItselfAndRegistersTheBuiltInProviders(t *testing.T) {
	p := NewAskPlugin()

	require.Equal(t, "ask", p.ID())
	require.True(t, p.DefaultEnabled())
	require.Nil(t, p.DependsOn())
	require.Nil(t, p.MCPTools())
	require.NotEmpty(t, p.ConfigSchema())

	for _, name := range []string{"ollama", "openai", "mistral"} {
		_, ok := p.providers[name]
		require.Truef(t, ok, "provider %q is not registered", name)
	}
}

func TestRegisteringANilProviderIsIgnored(t *testing.T) {
	p := NewAskPlugin()
	before := len(p.providers)

	p.RegisterProvider(nil)

	require.Len(t, p.providers, before)
}
