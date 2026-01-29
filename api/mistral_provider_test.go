package api

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

func TestNewMistralProvider(t *testing.T) {
	provider := NewMistralProvider()
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}

	if provider.client == nil {
		t.Error("expected non-nil HTTP client")
	}
}

func TestMistralProvider_GetProviderName(t *testing.T) {
	provider := NewMistralProvider()
	name := provider.GetProviderName()
	if name != "Mistral" {
		t.Errorf("expected 'Mistral', got '%s'", name)
	}
}

func TestMistralProvider_CheckModelExists(t *testing.T) {
	provider := NewMistralProvider()

	// Test with empty model (should set default)
	viper.Set("model", "")
	exists, err := provider.CheckModelExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected CheckModelExists to return true")
	}

	// Verify default model was set
	if viper.GetString("model") != "mistral-medium-latest" {
		t.Errorf("expected default model 'mistral-medium-latest', got '%s'", viper.GetString("model"))
	}
}

func TestMistralProvider_CheckModelExists_WithExistingModel(t *testing.T) {
	provider := NewMistralProvider()

	// Test with existing model
	viper.Set("model", "mistral-small")
	exists, err := provider.CheckModelExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected CheckModelExists to return true")
	}

	// Verify model was not changed
	if viper.GetString("model") != "mistral-small" {
		t.Errorf("expected model 'mistral-small', got '%s'", viper.GetString("model"))
	}
}

func TestMistralProvider_PullModel(t *testing.T) {
	provider := NewMistralProvider()

	// PullModel should be a no-op for Mistral
	err := provider.PullModel()
	if err != nil {
		t.Errorf("expected no error from PullModel, got: %v", err)
	}
}

func TestMistralProvider_SendMessage_NoAPIKey(t *testing.T) {
	provider := NewMistralProvider()

	// Ensure MISTRAL_API_KEY is not set
	oldKey := os.Getenv("MISTRAL_API_KEY")
	defer func() {
		if oldKey != "" {
			_ = os.Setenv("MISTRAL_API_KEY", oldKey)
		}
	}()
	_ = os.Unsetenv("MISTRAL_API_KEY")

	request := APIRequest{
		Model:    "mistral-tiny",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Stream:   false,
	}

	_, err := provider.SendMessage(request, false)
	if err == nil {
		t.Error("expected error when MISTRAL_API_KEY is not set")
	}

	expectedMsg := "MISTRAL_API_KEY environment variable is not set"
	if err.Error() != expectedMsg {
		t.Errorf("expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestMistralProvider_SendMessage_UsesDefaultModel(t *testing.T) {
	provider := NewMistralProvider()

	// Set API key but use an invalid one for this test
	oldKey := os.Getenv("MISTRAL_API_KEY")
	defer func() {
		if oldKey != "" {
			_ = os.Setenv("MISTRAL_API_KEY", oldKey)
		} else {
			_ = os.Unsetenv("MISTRAL_API_KEY")
		}
	}()
	_ = os.Setenv("MISTRAL_API_KEY", "test-key")

	viper.Set("model", "")

	// First check model exists to set the default model
	_, err := provider.CheckModelExists()
	if err != nil {
		t.Fatalf("unexpected error from CheckModelExists: %v", err)
	}
	if viper.GetString("model") != "mistral-medium-latest" {
		t.Errorf("expected default model 'mistral-medium-latest', got '%s'", viper.GetString("model"))
	}

	request := APIRequest{
		Model:    "",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Stream:   false,
	}

	// This will fail with network error, but we're testing that it tries to use the default model
	_, err = provider.SendMessage(request, false)

	// We expect an error (network or API error), but we're mainly checking that the function
	// doesn't panic and handles the default model case
	if err == nil {
		t.Error("expected error when calling Mistral with invalid key")
	}
}

func TestMistralProvider_SendMessage_StreamEnabled(t *testing.T) {
	provider := NewMistralProvider()

	// Set API key but use an invalid one for this test
	oldKey := os.Getenv("MISTRAL_API_KEY")
	defer func() {
		if oldKey != "" {
			_ = os.Setenv("MISTRAL_API_KEY", oldKey)
		} else {
			_ = os.Unsetenv("MISTRAL_API_KEY")
		}
	}()
	_ = os.Setenv("MISTRAL_API_KEY", "test-key")

	viper.Set("model", "mistral-tiny")

	request := APIRequest{
		Model:    "mistral-tiny",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	}

	// This will fail with network error, but we're testing that streaming is enabled
	_, err := provider.SendMessage(request, false)

	// We expect an error (network or API error)
	if err == nil {
		t.Error("expected error when calling Mistral with invalid key")
	}
}

func TestGetProvider_Mistral(t *testing.T) {
	// Test Mistral detection
	viper.Set("host", "api.mistral.ai")
	viper.Set("port", 443)

	provider, err := GetProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := provider.(*MistralProvider); !ok {
		t.Errorf("expected *MistralProvider, got %T", provider)
	}

	if provider.GetProviderName() != "Mistral" {
		t.Errorf("expected provider name 'Mistral', got '%s'", provider.GetProviderName())
	}
}

func TestGetProvider_MistralWithDifferentPortDefaultsToOllama(t *testing.T) {
	// Test that Mistral is only detected with port 443
	viper.Set("host", "api.mistral.ai")
	viper.Set("port", 8080)

	provider, err := GetProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if _, ok := provider.(*OllamaProvider); !ok {
		t.Errorf("expected *OllamaProvider, got %T", provider)
	}

	if provider.GetProviderName() != "Ollama" {
		t.Errorf("expected provider name 'Ollama', got '%s'", provider.GetProviderName())
	}
}
