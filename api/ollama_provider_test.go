package api

import (
	"testing"
)

func TestNewOllamaProvider(t *testing.T) {
	provider := NewOllamaProvider()
	if provider == nil {
		t.Error("expected non-nil provider")
	}
}

func TestOllamaProvider_GetProviderName(t *testing.T) {
	provider := NewOllamaProvider()
	name := provider.GetProviderName()
	if name != "Ollama" {
		t.Errorf("expected 'Ollama', got '%s'", name)
	}
}

func TestModelExists_OllamaProvider(t *testing.T) {
	tests := []struct {
		name      string
		models    []tagsModel
		modelName string
		expected  bool
	}{
		{
			name:      "exact tag match",
			models:    []tagsModel{{Name: "mistral:latest"}},
			modelName: "mistral:latest",
			expected:  true,
		},
		{
			name:      "base name matches tagged model",
			models:    []tagsModel{{Name: "mistral:latest"}},
			modelName: "mistral",
			expected:  true,
		},
		{
			name:      "base name matches untagged model",
			models:    []tagsModel{{Name: "mistral"}},
			modelName: "mistral",
			expected:  true,
		},
		{
			name:      "specific tag missing when only base exists",
			models:    []tagsModel{{Name: "mistral"}},
			modelName: "mistral:7b",
			expected:  false,
		},
		{
			name:      "specific tag missing when other tag exists",
			models:    []tagsModel{{Name: "mistral:latest"}},
			modelName: "mistral:7b",
			expected:  false,
		},
		{
			name:      "case insensitive base match",
			models:    []tagsModel{{Name: "MISTRAL:latest"}},
			modelName: "mistral",
			expected:  true,
		},
		{
			name:      "empty model name",
			models:    []tagsModel{{Name: "mistral"}},
			modelName: "",
			expected:  false,
		},
		{
			name:      "whitespace only model name",
			models:    []tagsModel{{Name: "mistral"}},
			modelName: "   ",
			expected:  false,
		},
		{
			name:      "empty models list",
			models:    []tagsModel{},
			modelName: "mistral",
			expected:  false,
		},
		{
			name:      "model with empty name in list",
			models:    []tagsModel{{Name: ""}, {Name: "mistral"}},
			modelName: "mistral",
			expected:  true,
		},
		{
			name:      "model with whitespace name in list",
			models:    []tagsModel{{Name: "  "}, {Name: "mistral"}},
			modelName: "mistral",
			expected:  true,
		},
		{
			name:      "multiple models with exact match",
			models:    []tagsModel{{Name: "llama"}, {Name: "mistral:latest"}, {Name: "gpt"}},
			modelName: "mistral:latest",
			expected:  true,
		},
		{
			name:      "multiple models with base match",
			models:    []tagsModel{{Name: "llama"}, {Name: "mistral:latest"}, {Name: "gpt"}},
			modelName: "mistral",
			expected:  true,
		},
		{
			name:      "no match in multiple models",
			models:    []tagsModel{{Name: "llama"}, {Name: "gpt"}},
			modelName: "mistral",
			expected:  false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := modelExists(test.models, test.modelName); got != test.expected {
				t.Errorf("expected %v, got %v", test.expected, got)
			}
		})
	}
}
