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

type MistralProvider struct{}

func NewMistralProvider() *MistralProvider { return &MistralProvider{} }

func (p *MistralProvider) Name() string { return "mistral" }

type mistralChatCompletionRequest struct {
	Model    string           `json:"model"`
	Messages []mistralMessage `json:"messages"`
	Stream   bool             `json:"stream"`
}

type mistralMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type mistralChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

func (p *MistralProvider) Send(ctx context.Context, req AskRequest) (AskResponse, error) {
	apiKey := strings.TrimSpace(os.Getenv("MISTRAL_API_KEY"))
	if apiKey == "" {
		return AskResponse{}, fmt.Errorf("MISTRAL_API_KEY environment variable is not set")
	}
	if strings.TrimSpace(req.Host) == "" || req.Port == 0 {
		return AskResponse{}, fmt.Errorf("mistral requires host and port to be set")
	}
	scheme := "http"
	if req.Port == 443 {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s:%d/v1/chat/completions", scheme, req.Host, req.Port)

	rawMessages := buildMessages(req)
	messages := make([]mistralMessage, 0, len(rawMessages))
	for _, msg := range rawMessages {
		role := strings.TrimSpace(msg["role"])
		content := strings.TrimSpace(msg["content"])
		if role == "" || content == "" {
			continue
		}
		messages = append(messages, mistralMessage{Role: role, Content: content})
	}

	mistralReq := mistralChatCompletionRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   false,
	}
	body, err := json.Marshal(mistralReq)
	if err != nil {
		return AskResponse{}, err
	}

	reqCtx, cancel := withTimeout(ctx, req.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return AskResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: req.Timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return AskResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return AskResponse{}, fmt.Errorf("mistral error: %s - %s", resp.Status, strings.TrimSpace(string(errBody)))
	}

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return AskResponse{}, err
	}
	var mistralResp mistralChatCompletionResponse
	if err := json.Unmarshal(respBody, &mistralResp); err != nil {
		return AskResponse{}, err
	}
	if len(mistralResp.Choices) == 0 {
		return AskResponse{}, fmt.Errorf("mistral response has no choices")
	}
	return AskResponse{Text: mistralResp.Choices[0].Message.Content}, nil
}

func (p *MistralProvider) SendStream(ctx context.Context, req AskRequest, onChunk func(string)) (AskResponse, error) {
	apiKey := strings.TrimSpace(os.Getenv("MISTRAL_API_KEY"))
	if apiKey == "" {
		return AskResponse{}, fmt.Errorf("MISTRAL_API_KEY environment variable is not set")
	}
	if strings.TrimSpace(req.Host) == "" || req.Port == 0 {
		return AskResponse{}, fmt.Errorf("mistral requires host and port to be set")
	}
	scheme := "http"
	if req.Port == 443 {
		scheme = "https"
	}
	url := fmt.Sprintf("%s://%s:%d/v1/chat/completions", scheme, req.Host, req.Port)

	rawMessages := buildMessages(req)
	messages := make([]mistralMessage, 0, len(rawMessages))
	for _, msg := range rawMessages {
		role := strings.TrimSpace(msg["role"])
		content := strings.TrimSpace(msg["content"])
		if role == "" || content == "" {
			continue
		}
		messages = append(messages, mistralMessage{Role: role, Content: content})
	}

	mistralReq := mistralChatCompletionRequest{
		Model:    req.Model,
		Messages: messages,
		Stream:   true,
	}
	body, err := json.Marshal(mistralReq)
	if err != nil {
		return AskResponse{}, err
	}

	reqCtx, cancel := withTimeout(ctx, req.Timeout)
	defer cancel()
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return AskResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Timeout: req.Timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return AskResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return AskResponse{}, fmt.Errorf("mistral error: %s - %s", resp.Status, strings.TrimSpace(string(errBody)))
	}

	var full strings.Builder
	buf := make([]byte, 4096)
	leftover := ""
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			chunk := leftover + string(buf[:n])
			lines := strings.Split(chunk, "\n")
			if !strings.HasSuffix(chunk, "\n") {
				leftover = lines[len(lines)-1]
				lines = lines[:len(lines)-1]
			} else {
				leftover = ""
			}
			for _, line := range lines {
				line = strings.TrimSpace(line)
				if line == "" || strings.HasPrefix(line, ":") {
					continue
				}
				if line == "data: [DONE]" {
					return AskResponse{Text: full.String()}, nil
				}
				if strings.HasPrefix(line, "data: ") {
					jsonData := strings.TrimPrefix(line, "data: ")
					var streamResp struct {
						Choices []struct {
							Delta struct {
								Content string `json:"content"`
							} `json:"delta"`
						} `json:"choices"`
					}
					if err := json.Unmarshal([]byte(jsonData), &streamResp); err != nil {
						continue
					}
					if len(streamResp.Choices) > 0 {
						delta := streamResp.Choices[0].Delta.Content
						if delta != "" {
							onChunk(delta)
							full.WriteString(delta)
						}
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return AskResponse{}, err
		}
	}
	return AskResponse{Text: full.String()}, nil
}
