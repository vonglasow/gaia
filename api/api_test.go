package api

import (
	"fmt"
	"os"
	"runtime"
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
