package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"github.com/spf13/viper"
)

// MistralProvider implements the Provider interface for Mistral AI
type MistralProvider struct {
	client *http.Client
}

// mistralChatCompletionRequest is the request structure for Mistral API
type mistralChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// mistralChatCompletionResponse is the response structure from Mistral API (non-streaming)
type mistralChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// mistralStreamResponse is the response structure for Mistral streaming API
type mistralStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// NewMistralProvider creates a new Mistral provider
func NewMistralProvider() *MistralProvider {
	return &MistralProvider{
		client: &http.Client{},
	}
}

// GetProviderName returns the name of the provider
func (p *MistralProvider) GetProviderName() string {
	return "Mistral"
}

// CheckModelExists always returns true for Mistral since models are validated server-side
func (p *MistralProvider) CheckModelExists() (bool, error) {
	// Mistral doesn't require pre-checking model existence
	// The API will return an error if the model doesn't exist
	modelName := viper.GetString("model")
	if modelName == "" {
		// Default Mistral model
		viper.Set("model", "mistral-medium-latest")
	}
	return true, nil
}

// PullModel is a no-op for Mistral since models are hosted remotely
func (p *MistralProvider) PullModel() error {
	// Mistral models don't need to be pulled
	return nil
}

// SendMessage sends a message to Mistral and returns the response with streaming support
func (p *MistralProvider) SendMessage(request APIRequest, printResponse bool) (string, error) {
	apiKey := os.Getenv("MISTRAL_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("MISTRAL_API_KEY environment variable is not set")
	}

	modelName := request.Model
	if modelName == "" {
		modelName = viper.GetString("model")
		if modelName == "" {
			modelName = "mistral-medium-latest"
		}
	}

	mistralRequest := mistralChatCompletionRequest{
		Model:    modelName,
		Messages: request.Messages,
		Stream:   request.Stream,
	}

	requestBody, err := json.Marshal(mistralRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal Mistral request: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://api.mistral.ai/v1/chat/completions",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create Mistral request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call Mistral API: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close Mistral response body: %v\n", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("mistral API error: %s - %s", resp.Status, string(errBody))
	}

	var content string
	if request.Stream {
		content, err = p.handleStreamingResponse(resp.Body, printResponse)
	} else {
		content, err = p.handleNonStreamingResponse(resp.Body, printResponse)
	}

	if err != nil {
		return "", err
	}

	if printResponse {
		fmt.Println()
	}

	return content, nil
}

// handleStreamingResponse processes Mistral streaming responses (SSE format)
func (p *MistralProvider) handleStreamingResponse(body io.Reader, printResponse bool) (string, error) {
	var contentBuilder bytes.Buffer
	buf := make([]byte, 4096)
	leftover := ""

	for {
		n, err := body.Read(buf)
		if n > 0 {
			chunk := leftover + string(buf[:n])
			lines := bytes.Split([]byte(chunk), []byte("\n"))

			// Keep the last incomplete line for next iteration
			if len(lines) > 0 && !bytes.HasSuffix([]byte(chunk), []byte("\n")) {
				leftover = string(lines[len(lines)-1])
				lines = lines[:len(lines)-1]
			} else {
				leftover = ""
			}

			for _, line := range lines {
				line = bytes.TrimSpace(line)
				if len(line) == 0 {
					continue
				}

				// Skip SSE comments and check for done signal
				if bytes.HasPrefix(line, []byte(":")) {
					continue
				}
				if bytes.Equal(line, []byte("data: [DONE]")) {
					break
				}

				// Parse SSE data line
				if bytes.HasPrefix(line, []byte("data: ")) {
					jsonData := bytes.TrimPrefix(line, []byte("data: "))
					var streamResp mistralStreamResponse
					if err := json.Unmarshal(jsonData, &streamResp); err != nil {
						// Ignore parse errors for incomplete chunks
						continue
					}

					if len(streamResp.Choices) > 0 {
						delta := streamResp.Choices[0].Delta.Content
						if delta != "" {
							if printResponse {
								fmt.Print(delta)
							}
							contentBuilder.WriteString(delta)
						}
					}
				}
			}
		}

		if err == io.EOF {
			break
		}
		if err != nil {
			return "", fmt.Errorf("failed to read streaming response: %w", err)
		}
	}

	return contentBuilder.String(), nil
}

// handleNonStreamingResponse processes Mistral non-streaming responses
func (p *MistralProvider) handleNonStreamingResponse(body io.Reader, printResponse bool) (string, error) {
	respBody, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("failed to read Mistral response: %w", err)
	}

	var mistralResp mistralChatCompletionResponse
	if err := json.Unmarshal(respBody, &mistralResp); err != nil {
		return "", fmt.Errorf("failed to decode Mistral response: %w", err)
	}

	if len(mistralResp.Choices) == 0 {
		return "", fmt.Errorf("mistral response has no choices")
	}

	content := mistralResp.Choices[0].Message.Content

	if printResponse {
		fmt.Print(content)
	}

	return content, nil
}
