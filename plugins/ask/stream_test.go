package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// Streaming is the path a person actually watches.

// ollamaChatAt stands in for an Ollama that already holds the model.
func ollamaChatAt(t *testing.T, model string, chat http.HandlerFunc) (string, int) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			w.Header().Set("Content-Type", "application/json")
			// Encoded rather than formatted.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"models": []map[string]string{{"name": model}},
			})
		case "/api/chat":
			chat(w, r)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	return hostAndPortOf(t, server)
}

func ollamaRequest(host string, port int) AskRequest {
	return AskRequest{Host: host, Port: port, Model: "llama3.1", Message: "a question", Timeout: 5 * time.Second}
}

func TestOllamaSendReadsTheAnswerBack(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"message":{"content":"an answer"},"done":true}`))
	})

	resp, err := NewOllamaProvider().Send(context.Background(), ollamaRequest(host, port))

	require.NoError(t, err)
	require.Equal(t, "an answer", resp.Text)
}

func TestOllamaSendReportsAnErrorStatusWithItsBody(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"model is loading"}`))
	})

	_, err := NewOllamaProvider().Send(context.Background(), ollamaRequest(host, port))

	require.ErrorContains(t, err, "status=500")
	require.ErrorContains(t, err, "model is loading",
		"the body is in the message because it is the only thing that says what went wrong")
}

func TestOllamaSendReportsAnAnswerItCannotDecode(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{not json`))
	})

	_, err := NewOllamaProvider().Send(context.Background(), ollamaRequest(host, port))

	require.Error(t, err)
}

// Every chunk must reach the caller, and the text returned must be their concatenation.
func TestOllamaStreamEmitsEveryChunkAndReturnsTheWhole(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-ndjson")
		for _, part := range []string{"Hello", ", ", "world"} {
			// Encode writes its own trailing newline.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"message": map[string]string{"content": part},
				"done":    false,
			})
		}
		_, _ = w.Write([]byte(`{"message":{"content":""},"done":true}` + "\n"))
	})

	var seen []string
	resp, err := NewOllamaProvider().SendStream(context.Background(), ollamaRequest(host, port),
		func(chunk string) { seen = append(seen, chunk) })

	require.NoError(t, err)
	require.Equal(t, []string{"Hello", ", ", "world"}, seen)
	require.Equal(t, "Hello, world", resp.Text)
	require.Equal(t, strings.Join(seen, ""), resp.Text,
		"what was shown and what was returned are the same text")
}

func TestOllamaStreamStopsAtTheDoneFlag(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"kept"},"done":true}` + "\n"))
		_, _ = w.Write([]byte(`{"message":{"content":"after the end"},"done":false}` + "\n"))
	})

	var seen []string
	resp, err := NewOllamaProvider().SendStream(context.Background(), ollamaRequest(host, port),
		func(chunk string) { seen = append(seen, chunk) })

	require.NoError(t, err)
	require.Equal(t, []string{"kept"}, seen)
	require.Equal(t, "kept", resp.Text)
}

// A stream that simply stops is the ordinary case for a connection closed cleanly.
func TestOllamaStreamEndsOnEndOfInput(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"partial"},"done":false}` + "\n"))
	})

	resp, err := NewOllamaProvider().SendStream(context.Background(), ollamaRequest(host, port), func(string) {})

	require.NoError(t, err)
	require.Equal(t, "partial", resp.Text)
}

func TestOllamaStreamReportsAChunkItCannotDecode(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"fine"},"done":false}` + "\n"))
		_, _ = w.Write([]byte("{not json\n"))
	})

	_, err := NewOllamaProvider().SendStream(context.Background(), ollamaRequest(host, port), func(string) {})

	require.Error(t, err, "a malformed chunk mid-stream is not a clean end")
}

func TestOllamaStreamReportsAnErrorStatus(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("busy"))
	})

	_, err := NewOllamaProvider().SendStream(context.Background(), ollamaRequest(host, port), func(string) {})

	require.ErrorContains(t, err, "status=503")
}

func TestOpenAIStreamEmitsEveryChunkAndReturnsTheWhole(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, part := range []string{"Hello", ", ", "world"} {
			// Server-sent events are not JSON on the wire, so the frame is built by hand.
			delta, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{"delta": map[string]string{"content": part}}},
			})
			// nosemgrep: go.lang.security.audit.xss.no-io-writestring-to-responsewriter.no-io-writestring-to-responsewriter
			_, _ = io.WriteString(w, "data: "+string(delta)+"\n\n")
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	host, port := hostAndPortOf(t, server)
	var seen []string
	resp, err := NewOpenAIProvider().SendStream(context.Background(),
		AskRequest{Host: host, Port: port, Model: "gpt-4o", Message: "a question", Timeout: 5 * time.Second},
		func(chunk string) { seen = append(seen, chunk) })

	require.NoError(t, err)
	require.Equal(t, "Hello, world", resp.Text)
	require.Equal(t, strings.Join(seen, ""), resp.Text)
}

func TestOpenAIStreamRefusesToStartWithoutAKey(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "")

	_, err := NewOpenAIProvider().SendStream(context.Background(),
		AskRequest{Host: "localhost", Port: 443, Model: "gpt-4o"}, func(string) {})

	require.ErrorContains(t, err, "OPENAI_API_KEY")
}

func TestOpenAIStreamRefusesAnIncompleteEndpoint(t *testing.T) {
	t.Setenv("OPENAI_API_KEY", "test-key")

	_, err := NewOpenAIProvider().SendStream(context.Background(),
		AskRequest{Model: "gpt-4o"}, func(string) {})

	require.ErrorContains(t, err, "host and port")
}

func TestMistralStreamRefusesToStartWithoutAKey(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "")

	_, err := NewMistralProvider().SendStream(context.Background(),
		AskRequest{Host: "localhost", Port: 443, Model: "mistral-large"}, func(string) {})

	require.ErrorContains(t, err, "MISTRAL_API_KEY")
}

func TestMistralStreamsTheAnswerChunkByChunk(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		for _, part := range []string{"Hello", ", ", "world"} {
			delta, _ := json.Marshal(map[string]any{
				"choices": []map[string]any{{"delta": map[string]string{"content": part}}},
			})
			// nosemgrep: go.lang.security.audit.xss.no-io-writestring-to-responsewriter.no-io-writestring-to-responsewriter
			_, _ = io.WriteString(w, "data: "+string(delta)+"\n\n")
		}
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer server.Close()

	host, port := hostAndPortOf(t, server)
	var seen []string
	resp, err := NewMistralProvider().SendStream(context.Background(),
		mistralRequest(host, port), func(chunk string) { seen = append(seen, chunk) })

	require.NoError(t, err)
	require.Equal(t, "Hello, world", resp.Text)
	require.Equal(t, strings.Join(seen, ""), resp.Text)
}

// A frame that is not the answer must not end up in it.
func TestMistralStreamSkipsCommentsAndFramesItCannotRead(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		// nosemgrep: go.lang.security.audit.xss.no-io-writestring-to-responsewriter.no-io-writestring-to-responsewriter
		_, _ = io.WriteString(w, ": keep-alive\ndata: not json\ndata: {\"choices\":[]}\n")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"only this\"}}]}\ndata: [DONE]\n"))
	}))
	defer server.Close()

	host, port := hostAndPortOf(t, server)
	resp, err := NewMistralProvider().SendStream(context.Background(),
		mistralRequest(host, port), func(string) {})

	require.NoError(t, err)
	require.Equal(t, "only this", resp.Text)
}

func TestMistralStreamReportsAnErrorStatus(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte(`{"message":"unauthorized"}`))
	}))
	defer server.Close()

	host, port := hostAndPortOf(t, server)
	_, err := NewMistralProvider().SendStream(context.Background(),
		mistralRequest(host, port), func(string) {})

	require.ErrorContains(t, err, "401")
}

// A stream that ends without [DONE] still has to give back what arrived.
func TestMistralStreamThatEndsWithoutADoneFrameKeepsWhatItRead(t *testing.T) {
	t.Setenv("MISTRAL_API_KEY", "test-key")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = w.Write([]byte("data: {\"choices\":[{\"delta\":{\"content\":\"half an answer\"}}]}\n"))
	}))
	defer server.Close()

	host, port := hostAndPortOf(t, server)
	resp, err := NewMistralProvider().SendStream(context.Background(),
		mistralRequest(host, port), func(string) {})

	require.NoError(t, err)
	require.Equal(t, "half an answer", resp.Text)
}

func mistralRequest(host string, port int) AskRequest {
	return AskRequest{
		Host: host, Port: port, Model: "mistral-large",
		Message: "a question", Timeout: 5 * time.Second,
	}
}

// --- sanitising before anything leaves the machine ------------------------

// Off by default, and off means untouched.
func TestApplySanitizeLeavesTheRequestAloneWhenItIsOff(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	req := AskRequest{SystemPrompt: "be brief", Message: "my email is someone@example.com"}
	var errOut bytes.Buffer

	require.Equal(t, req, ApplySanitize(&errOut, req))
	require.Empty(t, errOut.String())
}

// Once on, the whole request is rewritten into a message list.
func TestApplySanitizeFoldsEverythingIntoTheMessageList(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("sanitize.enabled", true)
	viper.Set("sanitize.level", "light")

	var errOut bytes.Buffer
	out := ApplySanitize(&errOut, AskRequest{SystemPrompt: "be brief", Message: "a question"})

	require.Empty(t, out.SystemPrompt)
	require.Empty(t, out.Message)
	require.NotEmpty(t, out.Messages)

	var seen string
	for _, m := range out.Messages {
		seen += m.Role + ":" + m.Content + "|"
	}
	require.Contains(t, seen, "be brief")
	require.Contains(t, seen, "a question")
}

// What "sanitize" actually does is worth stating plainly.
func TestSanitisingRemovesNoiseAndNotPersonalData(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("sanitize.enabled", true)
	viper.Set("sanitize.level", "aggressive")

	var errOut bytes.Buffer
	out := ApplySanitize(&errOut, AskRequest{
		Messages: []ChatMessage{{
			Role:    "assistant",
			Content: "[DEBUG] starting up\nwrite to someone@example.com\nreal content",
		}},
		Message: "and now summarise",
	})

	require.Len(t, out.Messages, 2)
	require.NotContains(t, out.Messages[0].Content, "[DEBUG]",
		"a debug line is noise and goes")
	require.Contains(t, out.Messages[0].Content, "real content")
	require.Contains(t, out.Messages[0].Content, "someone@example.com",
		"the address stays: nothing here redacts personal data, whatever the level")
}

// The last user turn carries a further exemption.
func TestTheLastUserTurnIsOnlyTidied(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("sanitize.enabled", true)
	viper.Set("sanitize.level", "aggressive")

	var errOut bytes.Buffer
	out := ApplySanitize(&errOut, AskRequest{
		Message: "[DEBUG] not stripped here\n\n\n\nwrite to someone@example.com",
	})

	require.Len(t, out.Messages, 1)
	require.Contains(t, out.Messages[0].Content, "[DEBUG] not stripped here",
		"the filters that apply to every other turn do not apply to this one")
	require.NotContains(t, out.Messages[0].Content, "\n\n\n",
		"runs of blank lines are collapsed, which is the whole of what it does here")
}

func TestApplySanitizeReportsItsStatsWhenAsked(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("sanitize.enabled", true)
	viper.Set("sanitize.level", "light")
	viper.Set("sanitize.log_stats", true)

	var errOut bytes.Buffer
	ApplySanitize(&errOut, AskRequest{Message: "a question worth several tokens"})

	require.Contains(t, errOut.String(), "[sanitize]")
	require.Contains(t, errOut.String(), "tokens before=")
}

func TestAnUnknownSanitizeLevelFallsBackToLight(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("sanitize.enabled", true)
	viper.Set("sanitize.level", "  NOT-A-LEVEL  ")

	var errOut bytes.Buffer
	out := ApplySanitize(&errOut, AskRequest{Message: "a question"})

	require.NotEmpty(t, out.Messages,
		"a typo in the level must not turn sanitising into a request that says nothing")
}

func TestBuildMessagesForSanitizeKeepsTheOrderAndDropsTheBlanks(t *testing.T) {
	msgs := buildMessagesForSanitize(AskRequest{
		SystemPrompt: "be brief",
		Messages:     []ChatMessage{{Role: "user", Content: "kept"}, {Role: "", Content: "no role"}},
		Message:      "the last word",
	})

	require.Len(t, msgs, 3)
	require.Equal(t, "system", msgs[0].Role)
	require.Equal(t, "kept", msgs[1].Content)
	require.Equal(t, "the last word", msgs[2].Content)
}
