package api

import (
	"fmt"
	"os"
	"runtime"
	"strings"
	"testing"

	"github.com/spf13/viper"
)

func resetChatHistory() {
	chatHistory = []Message{}
}

func TestBuildRequestPayload_WithSystemRole(t *testing.T) {
	resetChatHistory()

	// Mock config
	viper.Set("model", "my-model") // Required
	viper.Set("systemrole", "admin")
	viper.Set("roles.admin", "Hello %s on %s")

	// Mock environment
	if err := os.Setenv("SHELL", "/bin/bash"); err != nil {
		t.Fatalf("failed to set env: %v", err)
	}

	req, err := buildRequestPayload("User message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Model != "my-model" {
		t.Errorf("expected model 'my-model', got %s", req.Model)
	}

	if len(req.Messages) != 2 {
		t.Fatalf("expected 2 message entries, got %d", len(req.Messages))
	}

	system := req.Messages[0]
	if system.Role != "system" {
		t.Errorf("expected first message role 'system', got %s", system.Role)
	}

	expectedContent := fmt.Sprintf("Hello %s on %s", "/bin/bash", runtime.GOOS)
	if system.Content != expectedContent {
		t.Errorf("system message mismatch\nexpected: %q\ngot: %q", expectedContent, system.Content)
	}

	user := req.Messages[1]
	if user.Role != "user" {
		t.Errorf("expected user role 'user', got %s", user.Role)
	}
	if user.Content != "User message" {
		t.Errorf("expected user content 'User message', got %s", user.Content)
	}
}

func TestBuildRequestPayload_DefaultSystemRole(t *testing.T) {
	resetChatHistory()

	viper.Set("model", "test-model")
	viper.Set("systemrole", "") // not set
	viper.Set("role", "")       // not set
	viper.Set("roles.default", "")

	req, err := buildRequestPayload("Hello")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if req.Messages[0].Role != "system" {
		t.Errorf("expected system role, got %s", req.Messages[0].Role)
	}

	if req.Messages[0].Content != "" {
		t.Errorf("expected empty system content, got %s", req.Messages[0].Content)
	}
}

func TestBuildRequestPayload_WithPreviousHistory(t *testing.T) {
	resetChatHistory()
	viper.Set("model", "history-model")
	viper.Set("systemrole", "admin")
	viper.Set("roles.admin", "Sys role")

	chatHistory = append(chatHistory, Message{
		Role:    "assistant",
		Content: "Prev response",
	})

	req, err := buildRequestPayload("New message")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(req.Messages) != 3 {
		t.Fatalf("expected 3 total messages (sys + 1 history + user), got %d", len(req.Messages))
	}

	if req.Messages[1].Role != "assistant" {
		t.Errorf("expected preserved history role 'assistant', got %s", req.Messages[1].Role)
	}
	if req.Messages[1].Content != "Prev response" {
		t.Errorf("expected preserved history content, got %s", req.Messages[1].Content)
	}
}

func TestModelExists(t *testing.T) {
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
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := modelExists(test.models, test.modelName); got != test.expected {
				t.Errorf("expected %v, got %v", test.expected, got)
			}
		})
	}
}

func TestGetChatHistory(t *testing.T) {
	resetChatHistory()

	// Test empty history
	history := GetChatHistory()
	if len(history) != 0 {
		t.Errorf("expected empty history, got %d messages", len(history))
	}

	// Add some messages
	chatHistory = append(chatHistory, Message{Role: "user", Content: "hello"})
	chatHistory = append(chatHistory, Message{Role: "assistant", Content: "hi"})

	// Test that we get a copy
	history = GetChatHistory()
	if len(history) != 2 {
		t.Errorf("expected 2 messages, got %d", len(history))
	}

	// Modify the returned copy
	history[0].Content = "modified"

	// Verify original is unchanged
	if chatHistory[0].Content != "hello" {
		t.Errorf("expected original to be unchanged, got %s", chatHistory[0].Content)
	}
}

func TestSetChatHistory(t *testing.T) {
	resetChatHistory()

	// Test with empty history
	SetChatHistory([]Message{})
	if len(chatHistory) != 0 {
		t.Errorf("expected empty history after setting empty, got %d", len(chatHistory))
	}

	// Test with messages
	newHistory := []Message{
		{Role: "user", Content: "test1"},
		{Role: "assistant", Content: "test2"},
	}
	SetChatHistory(newHistory)

	if len(chatHistory) != 2 {
		t.Errorf("expected 2 messages, got %d", len(chatHistory))
	}

	// Modify the original slice
	newHistory[0].Content = "modified"

	// Verify chatHistory is a copy and unchanged
	if chatHistory[0].Content != "test1" {
		t.Errorf("expected copy to be independent, got %s", chatHistory[0].Content)
	}
}

func TestClearChatHistory(t *testing.T) {
	// Test clearing empty history
	resetChatHistory()
	ClearChatHistory()
	if len(chatHistory) != 0 {
		t.Errorf("expected empty history, got %d", len(chatHistory))
	}

	// Test clearing non-empty history
	chatHistory = append(chatHistory, Message{Role: "user", Content: "test"})
	chatHistory = append(chatHistory, Message{Role: "assistant", Content: "response"})
	ClearChatHistory()

	if len(chatHistory) != 0 {
		t.Errorf("expected empty history after clear, got %d", len(chatHistory))
	}

	// Verify it's actually cleared and not just reassigned
	if chatHistory == nil {
		t.Error("expected non-nil slice after clear")
	}
}

func TestProgressModel_Init(t *testing.T) {
	model := &ProgressModel{}
	cmd := model.Init()
	if cmd != nil {
		t.Errorf("expected nil cmd from Init, got %v", cmd)
	}
}

func TestProgressModel_View(t *testing.T) {
	model := &ProgressModel{Done: false, Total: 100, Completed: 50}

	// Test in-progress view
	view := model.View()
	if view == "" {
		t.Error("expected non-empty view")
	}
	if !containsString(view, "Press 'q' to cancel") {
		t.Error("expected view to contain cancel message")
	}

	// Test done view
	model.Done = true
	view = model.View()
	if !containsString(view, "Download completed") {
		t.Error("expected view to contain completion message")
	}

	// Test with zero total (boundary case)
	model = &ProgressModel{Done: false, Total: 0, Completed: 0}
	view = model.View()
	if view == "" {
		t.Error("expected non-empty view for zero total")
	}
}

func containsString(s, substr string) bool {
	return strings.Contains(s, substr)
}
