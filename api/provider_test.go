package api

import (
	"fmt"
	"testing"

	"github.com/spf13/viper"
)

func TestGetProvider_Ollama(t *testing.T) {
	// Configure for Ollama
	viper.Set("host", "localhost")
	viper.Set("port", 11434)

	provider, err := GetProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.GetProviderName() != "Ollama" {
		t.Errorf("expected Ollama provider, got %s", provider.GetProviderName())
	}

	if _, ok := provider.(*OllamaProvider); !ok {
		t.Error("expected OllamaProvider type")
	}
}

func TestGetProvider_OpenAI(t *testing.T) {
	// Configure for OpenAI
	viper.Set("host", "api.openai.com")
	viper.Set("port", 443)

	provider, err := GetProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.GetProviderName() != "OpenAI" {
		t.Errorf("expected OpenAI provider, got %s", provider.GetProviderName())
	}

	if _, ok := provider.(*OpenAIProvider); !ok {
		t.Error("expected OpenAIProvider type")
	}
}

func TestGetProvider_EmptyHost(t *testing.T) {
	viper.Set("host", "")
	viper.Set("port", 11434)

	_, err := GetProvider()
	if err == nil {
		t.Error("expected error for empty host")
	}

	expectedMsg := "configuration error: host is not set"
	if err.Error() != expectedMsg {
		t.Errorf("expected error '%s', got '%s'", expectedMsg, err.Error())
	}
}

func TestGetProvider_InvalidPort(t *testing.T) {
	viper.Set("host", "localhost")
	viper.Set("port", 0)

	_, err := GetProvider()
	if err == nil {
		t.Error("expected error for invalid port")
	}

	if err.Error() != fmt.Sprintf("configuration error: port is invalid (%d)", 0) {
		t.Errorf("expected port error, got: %s", err.Error())
	}
}

func TestGetProvider_NegativePort(t *testing.T) {
	viper.Set("host", "localhost")
	viper.Set("port", -1)

	_, err := GetProvider()
	if err == nil {
		t.Error("expected error for negative port")
	}
}

func TestGetProvider_CustomHostDefaultsToOllama(t *testing.T) {
	// Any host other than api.openai.com should default to Ollama
	viper.Set("host", "custom-server.local")
	viper.Set("port", 8080)

	provider, err := GetProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.GetProviderName() != "Ollama" {
		t.Errorf("expected Ollama provider for custom host, got %s", provider.GetProviderName())
	}
}

func TestGetProvider_OpenAIWithDifferentPortDefaultsToOllama(t *testing.T) {
	// api.openai.com with a port other than 443 should default to Ollama
	viper.Set("host", "api.openai.com")
	viper.Set("port", 8080)

	provider, err := GetProvider()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if provider.GetProviderName() != "Ollama" {
		t.Errorf("expected Ollama provider, got %s", provider.GetProviderName())
	}
}
