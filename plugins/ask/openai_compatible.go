// Package ask sends a question to a model, through one of its providers.
package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
)

// openAICompatible serves any provider speaking the OpenAI chat-completions API.
type openAICompatible struct {
	name   string
	envVar string
}

func NewOpenAIProvider() Provider {
	return &openAICompatible{name: "openai", envVar: "OPENAI_API_KEY"}
}

func NewMistralProvider() Provider {
	return &openAICompatible{name: "mistral", envVar: "MISTRAL_API_KEY"}
}

func (p *openAICompatible) Name() string { return p.name }

type openAIChatCompletionRequest struct {
	Model    string          `json:"model"`
	Messages []openAIMessage `json:"messages"`
	Stream   bool            `json:"stream"`
}

type openAIMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (p *openAICompatible) Send(ctx context.Context, req AskRequest) (AskResponse, error) {
	resp, cancel, err := p.post(ctx, req, false)
	if err != nil {
		return AskResponse{}, err
	}
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return AskResponse{}, err
	}
	var decoded openAIChatCompletionResponse
	if err := json.Unmarshal(body, &decoded); err != nil {
		return AskResponse{}, err
	}
	if len(decoded.Choices) == 0 {
		return AskResponse{}, fmt.Errorf("%s response has no choices", p.name)
	}
	return AskResponse{Text: decoded.Choices[0].Message.Content}, nil
}

func (p *openAICompatible) SendStream(ctx context.Context, req AskRequest, onChunk func(string)) (AskResponse, error) {
	resp, cancel, err := p.post(ctx, req, true)
	if err != nil {
		return AskResponse{}, err
	}
	defer cancel()
	defer func() { _ = resp.Body.Close() }()

	var full strings.Builder
	buf := make([]byte, 4096)
	leftover := ""
	for {
		n, readErr := resp.Body.Read(buf)
		if n > 0 {
			chunk := leftover + string(buf[:n])
			lines := strings.Split(chunk, "\n")
			// A read can stop mid-line; the tail waits for the rest.
			if !strings.HasSuffix(chunk, "\n") {
				leftover = lines[len(lines)-1]
				lines = lines[:len(lines)-1]
			} else {
				leftover = ""
			}
			for _, line := range lines {
				done := readSSELine(line, onChunk, &full)
				if done {
					return AskResponse{Text: full.String()}, nil
				}
			}
		}
		if readErr == io.EOF {
			break
		}
		if readErr != nil {
			return AskResponse{}, readErr
		}
	}
	return AskResponse{Text: full.String()}, nil
}

// readSSELine consumes one server-sent event, and reports the end of the stream.
func readSSELine(line string, onChunk func(string), full *strings.Builder) bool {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, ":") {
		return false
	}
	if line == "data: [DONE]" {
		return true
	}
	payload, ok := strings.CutPrefix(line, "data: ")
	if !ok {
		return false
	}
	var frame struct {
		Choices []struct {
			Delta struct {
				Content string `json:"content"`
			} `json:"delta"`
		} `json:"choices"`
	}
	if err := json.Unmarshal([]byte(payload), &frame); err != nil {
		return false
	}
	if len(frame.Choices) == 0 || frame.Choices[0].Delta.Content == "" {
		return false
	}
	onChunk(frame.Choices[0].Delta.Content)
	full.WriteString(frame.Choices[0].Delta.Content)
	return false
}

// post sends the question and hands back a response nobody has read yet, so
// Send and SendStream differ only in how they read it.
func (p *openAICompatible) post(ctx context.Context, req AskRequest, stream bool) (*http.Response, context.CancelFunc, error) {
	apiKey := strings.TrimSpace(os.Getenv(p.envVar))
	if apiKey == "" {
		return nil, nil, fmt.Errorf("%s environment variable is not set", p.envVar)
	}
	if strings.TrimSpace(req.Host) == "" || req.Port == 0 {
		return nil, nil, fmt.Errorf("%s requires host and port to be set", p.name)
	}

	body, err := json.Marshal(openAIChatCompletionRequest{
		Model:    req.Model,
		Messages: openAIMessages(req),
		Stream:   stream,
	})
	if err != nil {
		return nil, nil, err
	}

	reqCtx, cancel := withTimeout(ctx, req.Timeout)
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, p.url(req), bytes.NewReader(body))
	if err != nil {
		cancel()
		return nil, nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := (&http.Client{Timeout: req.Timeout}).Do(httpReq)
	if err != nil {
		cancel()
		return nil, nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		cancel()
		return nil, nil, fmt.Errorf("%s error: %s - %s", p.name, resp.Status, strings.TrimSpace(string(errBody)))
	}
	return resp, cancel, nil
}

// url: port 443 means somebody meant the real service, not a local stub.
func (p *openAICompatible) url(req AskRequest) string {
	scheme := "http"
	if req.Port == 443 {
		scheme = "https"
	}
	return fmt.Sprintf("%s://%s:%d/v1/chat/completions", scheme, req.Host, req.Port)
}

// openAIMessages drops anything half-written: the API refuses an empty role.
func openAIMessages(req AskRequest) []openAIMessage {
	raw := buildMessages(req)
	out := make([]openAIMessage, 0, len(raw))
	for _, msg := range raw {
		role := strings.TrimSpace(msg["role"])
		content := strings.TrimSpace(msg["content"])
		if role == "" || content == "" {
			continue
		}
		out = append(out, openAIMessage{Role: role, Content: content})
	}
	return out
}
