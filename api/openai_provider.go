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

// OpenAIProvider implements the Provider interface for OpenAI
type OpenAIProvider struct {
	client *http.Client
}

// openAIChatCompletionRequest is the request structure for OpenAI API
type openAIChatCompletionRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// openAIChatCompletionResponse is the response structure from OpenAI API (non-streaming)
type openAIChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

// openAIStreamResponse is the response structure for OpenAI streaming API
type openAIStreamResponse struct {
	Choices []struct {
		Delta struct {
			Content string `json:"content"`
		} `json:"delta"`
	} `json:"choices"`
}

// NewOpenAIProvider creates a new OpenAI provider
func NewOpenAIProvider() *OpenAIProvider {
	return &OpenAIProvider{
		client: &http.Client{},
	}
}

// GetProviderName returns the name of the provider
func (p *OpenAIProvider) GetProviderName() string {
	return "OpenAI"
}

// CheckModelExists always returns true for OpenAI since models are validated server-side
func (p *OpenAIProvider) CheckModelExists() (bool, error) {
	// OpenAI doesn't require pre-checking model existence
	// The API will return an error if the model doesn't exist
	modelName := viper.GetString("model")
	if modelName == "" {
		// Default OpenAI model
		viper.Set("model", "gpt-4o-mini")
	}
	return true, nil
}

// PullModel is a no-op for OpenAI since models are hosted remotely
func (p *OpenAIProvider) PullModel() error {
	// OpenAI models don't need to be pulled
	return nil
}

// SendMessage sends a message to OpenAI and returns the response with streaming support
func (p *OpenAIProvider) SendMessage(request APIRequest, printResponse bool) (string, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return "", fmt.Errorf("OPENAI_API_KEY environment variable is not set")
	}

	modelName := request.Model
	if modelName == "" {
		modelName = viper.GetString("model")
		if modelName == "" {
			modelName = "gpt-4o-mini"
		}
	}

	openaiRequest := openAIChatCompletionRequest{
		Model:    modelName,
		Messages: request.Messages,
		Stream:   request.Stream,
	}

	requestBody, err := json.Marshal(openaiRequest)
	if err != nil {
		return "", fmt.Errorf("failed to marshal OpenAI request: %w", err)
	}

	req, err := http.NewRequest(
		http.MethodPost,
		"https://api.openai.com/v1/chat/completions",
		bytes.NewReader(requestBody),
	)
	if err != nil {
		return "", fmt.Errorf("failed to create OpenAI request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := p.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to call OpenAI API: %w", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close OpenAI response body: %v\n", err)
		}
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("OpenAI API error: %s - %s", resp.Status, string(errBody))
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

// handleStreamingResponse processes OpenAI streaming responses (SSE format)
func (p *OpenAIProvider) handleStreamingResponse(body io.Reader, printResponse bool) (string, error) {
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
					var streamResp openAIStreamResponse
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

// handleNonStreamingResponse processes OpenAI non-streaming responses
func (p *OpenAIProvider) handleNonStreamingResponse(body io.Reader, printResponse bool) (string, error) {
	respBody, err := io.ReadAll(body)
	if err != nil {
		return "", fmt.Errorf("failed to read OpenAI response: %w", err)
	}

	var openaiResp openAIChatCompletionResponse
	if err := json.Unmarshal(respBody, &openaiResp); err != nil {
		return "", fmt.Errorf("failed to decode OpenAI response: %w", err)
	}

	if len(openaiResp.Choices) == 0 {
		return "", fmt.Errorf("OpenAI response has no choices")
	}

	content := openaiResp.Choices[0].Message.Content

	if printResponse {
		fmt.Print(content)
	}

	return content, nil
}
