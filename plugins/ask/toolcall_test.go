package ask

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// The loop used to ask for JSON in a prompt and dig it out of free text.

func TestNoToolsMeansNoToolsField(t *testing.T) {
	var body map[string]any
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"message":{"content":"an answer"},"done":true}`))
	})

	_, err := NewOllamaProvider().Send(context.Background(), ollamaRequest(host, port))

	require.NoError(t, err)
	require.NotContains(t, body, "tools",
		"a plain question must not start announcing tools to the model")
}

func TestTheToolsOfferedReachTheModel(t *testing.T) {
	var body map[string]any
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"message":{"content":"an answer"},"done":true}`))
	})

	req := ollamaRequest(host, port)
	req.Tools = []ToolSpec{{
		Name:        "read_file",
		Description: "Read a file",
		Parameters: map[string]any{
			"type":       "object",
			"properties": map[string]any{"path": map[string]any{"type": "string"}},
			"required":   []string{"path"},
		},
	}}

	_, err := NewOllamaProvider().Send(context.Background(), req)
	require.NoError(t, err)

	tools, ok := body["tools"].([]any)
	require.True(t, ok)
	require.Len(t, tools, 1)

	tool := tools[0].(map[string]any)
	require.Equal(t, "function", tool["type"])
	fn := tool["function"].(map[string]any)
	require.Equal(t, "read_file", fn["name"])
	require.Equal(t, "Read a file", fn["description"])
	require.NotNil(t, fn["parameters"])
}

// A tool taking no arguments still needs a schema.
func TestAToolWithNoArgumentsStillCarriesASchema(t *testing.T) {
	rendered := toOllamaTools([]ToolSpec{{Name: "git_status", Description: "status"}})

	require.Len(t, rendered, 1)
	require.Equal(t, "object", rendered[0].Function.Parameters["type"])
	require.NotNil(t, rendered[0].Function.Parameters["properties"])
}

func TestNoSpecsRenderToNothing(t *testing.T) {
	require.Nil(t, toOllamaTools(nil))
	require.Nil(t, toOllamaTools([]ToolSpec{}))
}

func TestACallComesBackStructured(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"","tool_calls":[
			{"function":{"name":"read_file","arguments":{"path":"main.go"}}}
		]},"done":true}`))
	})

	resp, err := NewOllamaProvider().Send(context.Background(), ollamaRequest(host, port))

	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 1)
	require.Equal(t, "read_file", resp.ToolCalls[0].Name)

	path, err := resp.ToolCalls[0].ArgString("path")
	require.NoError(t, err)
	require.Equal(t, "main.go", path)
}

func TestSeveralCallsInOneTurnAllArrive(t *testing.T) {
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"message":{"content":"","tool_calls":[
			{"function":{"name":"read_file","arguments":{"path":"a.go"}}},
			{"function":{"name":"read_file","arguments":{"path":"b.go"}}}
		]},"done":true}`))
	})

	resp, err := NewOllamaProvider().Send(context.Background(), ollamaRequest(host, port))

	require.NoError(t, err)
	require.Len(t, resp.ToolCalls, 2)
}

// Some Ollama builds send the arguments as a JSON string rather than an object.
func TestArgumentsArriveAsAnObjectOrAsAString(t *testing.T) {
	asObject := parseToolCalls([]ollamaToolCall{jsonCall(t, `{"name":"t","arguments":{"path":"a.go"}}`)})
	require.Equal(t, "a.go", asObject[0].Arguments["path"])

	asString := parseToolCalls([]ollamaToolCall{jsonCall(t, `{"name":"t","arguments":"{\"path\":\"a.go\"}"}`)})
	require.Equal(t, "a.go", asString[0].Arguments["path"])
}

func jsonCall(t *testing.T, function string) ollamaToolCall {
	t.Helper()
	var call ollamaToolCall
	require.NoError(t, json.Unmarshal([]byte(`{"function":`+function+`}`), &call))
	return call
}

// Arguments that cannot be read become no arguments rather than an error.
func TestUnreadableArgumentsBecomeNoArguments(t *testing.T) {
	calls := parseToolCalls([]ollamaToolCall{jsonCall(t, `{"name":"t","arguments":"not json at all"}`)})

	require.Len(t, calls, 1)
	require.Empty(t, calls[0].Arguments)
}

func TestACallWithNoNameIsDropped(t *testing.T) {
	require.Empty(t, parseToolCalls([]ollamaToolCall{jsonCall(t, `{"name":"  ","arguments":{}}`)}))
	require.Nil(t, parseToolCalls(nil))
}

// --- reading what a model sent -------------------------------------------

// JSON has one number type, so a port written as 8080 arrives as a float.
func TestAnArgumentIsReadWhateverJSONTypeItArrivedAs(t *testing.T) {
	call := ToolCall{Name: "t", Arguments: map[string]any{
		"text":    "hello",
		"whole":   float64(8080),
		"decimal": 1.5,
		"flag":    true,
		"nested":  map[string]any{"a": 1},
	}}

	require.Equal(t, "hello", mustArg(t, call, "text"))
	require.Equal(t, "8080", mustArg(t, call, "whole"), "a whole number is not 8080.000000")
	require.Equal(t, "1.5", mustArg(t, call, "decimal"))
	require.Equal(t, "true", mustArg(t, call, "flag"))
	require.Contains(t, mustArg(t, call, "nested"), `"a":1`)
}

func mustArg(t *testing.T, call ToolCall, name string) string {
	t.Helper()
	value, err := call.ArgString(name)
	require.NoError(t, err)
	return value
}

// A missing argument is refused rather than guessed.
func TestAMissingArgumentIsAnError(t *testing.T) {
	call := ToolCall{Name: "read_file", Arguments: map[string]any{}}

	_, err := call.ArgString("path")

	require.ErrorContains(t, err, "missing argument")
	require.ErrorContains(t, err, "read_file", "the message names the tool that asked")
}

func TestANullArgumentCountsAsMissing(t *testing.T) {
	call := ToolCall{Name: "t", Arguments: map[string]any{"path": nil}}

	_, err := call.ArgString("path")

	require.Error(t, err)
}

func TestAFallbackIsUsedWhereADefaultBeatsAnError(t *testing.T) {
	call := ToolCall{Name: "t", Arguments: map[string]any{"path": "  "}}

	require.Equal(t, ".", call.ArgStringOr("path", "."))
	require.Equal(t, ".", call.ArgStringOr("absent", "."))
	require.Equal(t, "src", ToolCall{Arguments: map[string]any{"path": "src"}}.ArgStringOr("path", "."))
}

// --- the conversation a tool exchange produces ----------------------------

func TestAnAssistantTurnCarriesTheCallsItMade(t *testing.T) {
	messages := buildToolMessages(AskRequest{
		SystemPrompt: "you are an operator",
		Messages: []ChatMessage{
			{Role: "user", Content: "what is in main.go"},
			{Role: "assistant", ToolCalls: []ToolCall{{Name: "read_file", Arguments: map[string]any{"path": "main.go"}}}},
			{Role: "tool", ToolName: "read_file", Content: "package main"},
		},
	})

	require.Len(t, messages, 4)
	require.Equal(t, "system", messages[0]["role"])

	assistant := messages[2]
	calls, ok := assistant["tool_calls"].([]map[string]any)
	require.True(t, ok, "an assistant turn with no content is still a turn, because of its calls")
	require.Len(t, calls, 1)

	require.Equal(t, "read_file", messages[3]["tool_name"],
		"without it, a model with two calls in flight cannot tell which result is which")
}

func TestATurnWithNothingToSayIsDropped(t *testing.T) {
	messages := buildToolMessages(AskRequest{
		Messages: []ChatMessage{
			{Role: "user", Content: "kept"},
			{Role: "assistant", Content: "   "},
			{Role: "", Content: "no role"},
		},
	})

	require.Len(t, messages, 1)
	require.Equal(t, "kept", messages[0]["content"])
}

func TestTheQuestionComesLast(t *testing.T) {
	messages := buildToolMessages(AskRequest{
		Messages: []ChatMessage{{Role: "user", Content: "earlier"}},
		Message:  "and now this",
	})

	require.Len(t, messages, 2)
	require.Equal(t, "and now this", messages[1]["content"])
}

func TestACallWithNoArgumentsIsStillWellFormed(t *testing.T) {
	messages := buildToolMessages(AskRequest{
		Messages: []ChatMessage{{Role: "assistant", ToolCalls: []ToolCall{{Name: "git_status"}}}},
	})

	calls := messages[0]["tool_calls"].([]map[string]any)
	fn := calls[0]["function"].(map[string]any)
	require.NotNil(t, fn["arguments"], "a nil arguments field is not the same as an empty object")
}

func TestAToolExchangeSurvivesARoundTrip(t *testing.T) {
	var body map[string]any
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"message":{"content":"main.go holds package main"},"done":true}`))
	})

	req := AskRequest{
		Host: host, Port: port, Model: "llama3.1", Timeout: 5 * time.Second,
		Messages: []ChatMessage{
			{Role: "user", Content: "what is in main.go"},
			{Role: "assistant", ToolCalls: []ToolCall{{Name: "read_file", Arguments: map[string]any{"path": "main.go"}}}},
			{Role: "tool", ToolName: "read_file", Content: "package main"},
		},
		Tools: []ToolSpec{{Name: "read_file", Description: "Read a file"}},
	}

	resp, err := NewOllamaProvider().Send(context.Background(), req)

	require.NoError(t, err)
	require.Contains(t, resp.Text, "package main")

	sent := body["messages"].([]any)
	require.Len(t, sent, 3)
	require.Contains(t, sent[1].(map[string]any), "tool_calls")
	require.Equal(t, "read_file", sent[2].(map[string]any)["tool_name"])
}

// Ollama keeps a model five minutes by default: 18 GB for a 30B nobody is using.
func TestKeepAliveIsSentWhenItWasAskedFor(t *testing.T) {
	var body map[string]any
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"message":{"content":"ok"},"done":true}`))
	})

	req := ollamaRequest(host, port)
	req.KeepAlive = "30s"
	_, err := NewOllamaProvider().Send(context.Background(), req)

	require.NoError(t, err)
	require.Equal(t, "30s", body["keep_alive"])
}

// Absent by default, so gaia does not quietly change how long a model stays.
func TestNoKeepAliveMeansOllamaDecides(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	var body map[string]any
	host, port := ollamaChatAt(t, "llama3.1", func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &body)
		_, _ = w.Write([]byte(`{"message":{"content":"ok"},"done":true}`))
	})

	_, err := NewOllamaProvider().Send(context.Background(), ollamaRequest(host, port))

	require.NoError(t, err)
	require.NotContains(t, body, "keep_alive")
}

func TestTheConfiguredKeepAliveAppliesWhenTheRequestSaysNothing(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("ollama.keep_alive", "1m")

	require.Equal(t, "1m", KeepAlive(AskRequest{}))
	require.Equal(t, "0", KeepAlive(AskRequest{KeepAlive: "0"}),
		"what the run asked for wins, which is how --unload drops the model")
}
