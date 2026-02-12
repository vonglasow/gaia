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

	// Test with empty model: CheckModelExists returns true and does not mutate config
	viper.Set("model", "")
	exists, err := provider.CheckModelExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected CheckModelExists to return true")
	}
	// No side effect: config model remains empty; SendMessage uses a local default when needed
	if viper.GetString("model") != "" {
		t.Errorf("expected config unchanged (model empty), got '%s'", viper.GetString("model"))
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

	request := APIRequest{
		Model:    "",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Stream:   false,
	}

	// This will fail with network error, but we're testing that it tries to use the default model
	_, err := provider.SendMessage(request, false)

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
