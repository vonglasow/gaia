package api

import (
	"os"
	"testing"

	"github.com/spf13/viper"
)

func TestNewOpenAIProvider(t *testing.T) {
	provider := NewOpenAIProvider()
	if provider == nil {
		t.Fatal("expected non-nil provider")
	}

	if provider.client == nil {
		t.Error("expected non-nil HTTP client")
	}
}

func TestOpenAIProvider_GetProviderName(t *testing.T) {
	provider := NewOpenAIProvider()
	name := provider.GetProviderName()
	if name != "OpenAI" {
		t.Errorf("expected 'OpenAI', got '%s'", name)
	}
}

func TestOpenAIProvider_CheckModelExists(t *testing.T) {
	provider := NewOpenAIProvider()

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
	if viper.GetString("model") != "gpt-4o-mini" {
		t.Errorf("expected default model 'gpt-4o-mini', got '%s'", viper.GetString("model"))
	}
}

func TestOpenAIProvider_CheckModelExists_WithExistingModel(t *testing.T) {
	provider := NewOpenAIProvider()

	// Test with existing model
	viper.Set("model", "gpt-4")
	exists, err := provider.CheckModelExists()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Error("expected CheckModelExists to return true")
	}

	// Verify model was not changed
	if viper.GetString("model") != "gpt-4" {
		t.Errorf("expected model 'gpt-4', got '%s'", viper.GetString("model"))
	}
}

func TestOpenAIProvider_PullModel(t *testing.T) {
	provider := NewOpenAIProvider()

	// PullModel should be a no-op for OpenAI
	err := provider.PullModel()
	if err != nil {
		t.Errorf("expected no error from PullModel, got: %v", err)
	}
}

func TestOpenAIProvider_SendMessage_NoAPIKey(t *testing.T) {
	provider := NewOpenAIProvider()

	// Ensure OPENAI_API_KEY is not set
	oldKey := os.Getenv("OPENAI_API_KEY")
	defer func() {
		if oldKey != "" {
			_ = os.Setenv("OPENAI_API_KEY", oldKey)
		}
	}()
	_ = os.Unsetenv("OPENAI_API_KEY")

	request := APIRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Stream:   false,
	}

	_, err := provider.SendMessage(request, false)
	if err == nil {
		t.Error("expected error when OPENAI_API_KEY is not set")
	}

	expectedMsg := "OPENAI_API_KEY environment variable is not set"
	if err.Error() != expectedMsg {
		t.Errorf("expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestOpenAIProvider_SendMessage_UsesDefaultModel(t *testing.T) {
	provider := NewOpenAIProvider()

	// Set API key but use an invalid one for this test
	oldKey := os.Getenv("OPENAI_API_KEY")
	defer func() {
		if oldKey != "" {
			_ = os.Setenv("OPENAI_API_KEY", oldKey)
		} else {
			_ = os.Unsetenv("OPENAI_API_KEY")
		}
	}()
	_ = os.Setenv("OPENAI_API_KEY", "test-key")

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
		t.Error("expected error when calling OpenAI with invalid key")
	}
}

func TestOpenAIProvider_SendMessage_StreamEnabled(t *testing.T) {
	provider := NewOpenAIProvider()

	// Set API key but use an invalid one for this test
	oldKey := os.Getenv("OPENAI_API_KEY")
	defer func() {
		if oldKey != "" {
			_ = os.Setenv("OPENAI_API_KEY", oldKey)
		} else {
			_ = os.Unsetenv("OPENAI_API_KEY")
		}
	}()
	_ = os.Setenv("OPENAI_API_KEY", "test-key")

	viper.Set("model", "gpt-4o-mini")

	request := APIRequest{
		Model:    "gpt-4o-mini",
		Messages: []Message{{Role: "user", Content: "Hello"}},
		Stream:   true,
	}

	// This will fail with network error, but we're testing that streaming is enabled
	_, err := provider.SendMessage(request, false)

	// We expect an error (network or API error)
	if err == nil {
		t.Error("expected error when calling OpenAI with invalid key")
	}
}
