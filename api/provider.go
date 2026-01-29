package api

import (
	"fmt"
	"strings"

	"github.com/spf13/viper"
)

// Provider defines the interface for different AI service providers
type Provider interface {
	// CheckModelExists checks if the configured model exists
	CheckModelExists() (bool, error)

	// PullModel downloads the model if it doesn't exist (only for providers that support it)
	PullModel() error

	// SendMessage sends a message to the provider and returns the response
	// If printResponse is true, the response is printed to stdout as it streams
	SendMessage(request APIRequest, printResponse bool) (string, error)

	// GetProviderName returns the name of the provider
	GetProviderName() string
}

// GetProvider returns the appropriate provider based on configuration
func GetProvider() (Provider, error) {
	host := viper.GetString("host")
	port := viper.GetInt("port")

	if host == "" {
		return nil, fmt.Errorf("configuration error: host is not set")
	}
	if port <= 0 {
		return nil, fmt.Errorf("configuration error: port is invalid (%d)", port)
	}

	// Detect OpenAI provider
	if strings.Contains(host, "api.openai.com") && port == 443 {
		return NewOpenAIProvider(), nil
	}

	// Detect Mistral provider
	if strings.Contains(host, "api.mistral.ai") && port == 443 {
		return NewMistralProvider(), nil
	}

	// Default to Ollama provider
	return NewOllamaProvider(), nil
}

// checkAndPullIfRequired checks if the model exists and pulls it if necessary
func checkAndPullIfRequired() error {
	provider, err := GetProvider()
	if err != nil {
		return err
	}

	exists, err := provider.CheckModelExists()
	if err != nil {
		return err
	}

	if !exists {
		modelName := viper.GetString("model")
		fmt.Printf("Model %s not found, pulling...\n", modelName)
		return provider.PullModel()
	}

	return nil
}

// sendMessage sends a message using the configured provider
func sendMessage(msg string) (string, error) {
	return sendMessageInternal(msg, true)
}

// sendMessageInternal sends a message and optionally prints the response
func sendMessageInternal(msg string, printResponse bool) (string, error) {
	provider, err := GetProvider()
	if err != nil {
		return "", err
	}

	request, err := buildRequestPayload(msg)
	if err != nil {
		return "", err
	}

	responseContent, err := provider.SendMessage(request, printResponse)
	if err != nil {
		return "", err
	}

	// Add user message and assistant response to history
	chatHistory = append(chatHistory, Message{Role: "user", Content: msg})
	chatHistory = append(chatHistory, Message{Role: "assistant", Content: responseContent})

	return responseContent, nil
}
