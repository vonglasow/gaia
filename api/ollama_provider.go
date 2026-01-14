package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/spf13/viper"
)

// tagsModel represents a model in Ollama's tags response
type tagsModel struct {
	Name string `json:"name"`
}

// tagsResponse represents the response from Ollama's /api/tags endpoint
type tagsResponse struct {
	Models []tagsModel `json:"models"`
}

// OllamaProvider implements the Provider interface for Ollama
type OllamaProvider struct{}

// NewOllamaProvider creates a new Ollama provider
func NewOllamaProvider() *OllamaProvider {
	return &OllamaProvider{}
}

// GetProviderName returns the name of the provider
func (p *OllamaProvider) GetProviderName() string {
	return "Ollama"
}

// CheckModelExists checks if the configured model exists in Ollama
func (p *OllamaProvider) CheckModelExists() (bool, error) {
	host := viper.GetString("host")
	port := viper.GetInt("port")
	modelName := viper.GetString("model")

	if modelName == "" {
		return false, fmt.Errorf("configuration error: model name is not set")
	}

	url := fmt.Sprintf("http://%s:%d/api/tags", host, port)

	resp, err := http.Get(url)
	if err != nil {
		return false, fmt.Errorf("failed to connect to API server at %s:%d: %w. Please ensure the server is running", host, port, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("API server returned status %d: %s. Please check server configuration", resp.StatusCode, resp.Status)
	}

	var tagsResponse tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResponse); err != nil {
		return false, fmt.Errorf("failed to decode API response: %w. The server may be returning invalid data", err)
	}

	return modelExists(tagsResponse.Models, modelName), nil
}

// PullModel downloads the model using Ollama API with a progress bar
func (p *OllamaProvider) PullModel() error {
	host := viper.GetString("host")
	port := viper.GetInt("port")
	modelName := viper.GetString("model")
	if modelName == "" {
		return fmt.Errorf("configuration error: model name is not set")
	}

	pullURL := fmt.Sprintf("http://%s:%d/api/pull", host, port)
	pullDataBytes, err := json.Marshal(map[string]string{"name": modelName})
	if err != nil {
		return fmt.Errorf("failed to prepare pull request: %w", err)
	}

	resp, err := http.Post(pullURL, "application/json", bytes.NewBuffer(pullDataBytes))
	if err != nil {
		return fmt.Errorf("failed to connect to API server to pull model '%s': %w. Please ensure the server is running", modelName, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		// Try to read error message from response
		body, _ := io.ReadAll(resp.Body)
		bodyStr := strings.TrimSpace(string(body))
		if bodyStr != "" {
			return fmt.Errorf("failed to pull model '%s': API returned status %d: %s. Response: %s", modelName, resp.StatusCode, resp.Status, bodyStr)
		}
		return fmt.Errorf("failed to pull model '%s': API returned status %d: %s. The model may not exist or the server encountered an error", modelName, resp.StatusCode, resp.Status)
	}

	model := &ProgressModel{progress: progress.New(progress.WithWidth(50))}
	prg := tea.NewProgram(model)

	go func() {
		decoder := json.NewDecoder(resp.Body)
		for {
			var pullResponse struct {
				Completed int64 `json:"completed"`
				Total     int64 `json:"total"`
			}
			if err := decoder.Decode(&pullResponse); err != nil {
				if err != io.EOF {
					fmt.Fprintf(os.Stderr, "Warning: error decoding pull progress: %v\n", err)
				}
				break
			}
			prg.Send(struct {
				completed int64
				total     int64
			}{pullResponse.Completed, pullResponse.Total})
		}
		prg.Send("done")
	}()

	if _, err := prg.Run(); err != nil {
		return fmt.Errorf("error running progress UI: %v", err)
	}

	return nil
}

// SendMessage sends a message to Ollama and returns the response
func (p *OllamaProvider) SendMessage(request APIRequest, printResponse bool) (string, error) {
	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON request: %v", err)
	}

	host := viper.GetString("host")
	port := viper.GetInt("port")
	url := fmt.Sprintf("http://%s:%d/api/chat", host, port)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to connect to API server at %s:%d: %w. Please ensure the server is running", host, port, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API server returned status %d: %s. The request may be invalid or the server is experiencing issues", resp.StatusCode, resp.Status)
	}

	var responseBuilder strings.Builder
	decoder := json.NewDecoder(resp.Body)
	for {
		var apiResp APIResponse
		if err := decoder.Decode(&apiResp); err != nil {
			if err == io.EOF {
				break
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				break
			}
			return "", fmt.Errorf("failed to decode API response: %w. The server may be returning invalid or incomplete data", err)
		}

		if apiResp.Message != nil {
			if printResponse {
				fmt.Print(apiResp.Message.Content)
			}
			responseBuilder.WriteString(apiResp.Message.Content)
		}
	}
	if printResponse {
		fmt.Println()
	}

	return responseBuilder.String(), nil
}

// modelExists checks if a model exists in the list of models
func modelExists(models []tagsModel, modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}

	modelNameHasTag := strings.Contains(modelName, ":")
	modelNameBase := strings.Split(modelName, ":")[0]

	for _, model := range models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		if strings.EqualFold(name, modelName) {
			return true
		}
		if !modelNameHasTag {
			modelBase := strings.Split(name, ":")[0]
			if strings.EqualFold(modelBase, modelNameBase) {
				return true
			}
		}
	}

	return false
}
