package api

import (
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
)

// Helper function to safely close tea.Program for tests
// to avoid terminal issues.
func runTeaProgramWithTimeout(p *tea.Program, t *testing.T) {
	var wg sync.WaitGroup
	wg.Add(1)

	go func() {
		defer wg.Done()
		if _, err := p.Run(); err != nil {
			// This error is often expected in tests when p.Quit() is called
			// log.Printf("Bubbletea program exited with error (expected in test): %v", err)
		}
	}()

	time.Sleep(100 * time.Millisecond) // Give program time to init

	p.Quit() // Attempt graceful shutdown

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		// Program exited
	case <-time.After(1 * time.Second): // Reduced timeout
		t.Log("Bubbletea program did not quit in time via p.Quit(), attempting to send KeyMsg 'q'")
		p.Send(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune("q")})
		select {
		case <-done:
			// Program exited after 'q'
		case <-time.After(1 * time.Second):
			t.Error("Bubbletea program did not quit after 'q' key")
		}
	}
}

func TestChatSession_AddMessage(t *testing.T) {
	cs := NewChatSession()
	cs.AddMessage("user", "hello")
	cs.AddMessage("assistant", "world")

	expected := []Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "world"},
	}
	if !reflect.DeepEqual(cs.History, expected) {
		t.Errorf("History incorrect, got: %v, want: %v", cs.History, expected)
	}
}

func TestChatSession_GetHistory(t *testing.T) {
	cs := NewChatSession()
	cs.AddMessage("user", "test")
	history := cs.GetHistory()
	if len(history) != 1 || history[0].Content != "test" {
		t.Errorf("GetHistory failed, got: %v", history)
	}
}

func TestNewAPIClient(t *testing.T) {
	client := NewAPIClient("localhost", "1234", "test-model")
	if client.BaseURL != "http://localhost:1234/api" {
		t.Errorf("Unexpected BaseURL: got %s, want: %s", client.BaseURL, "http://localhost:1234/api")
	}
	if client.ModelName != "test-model" {
		t.Errorf("Unexpected ModelName: got %s, want: %s", client.ModelName, "test-model")
	}
	if client.HTTPClient == nil {
		t.Error("HTTPClient should not be nil")
	}
}

func TestAPIClient_CheckAndPullModel_ModelExists(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models": [{"name": "test-model:latest"}]}`))
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
			t.Fatalf("Unexpected request to %s", r.URL.Path)
		}
	}))
	defer server.Close()

	host, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}

	client := NewAPIClient(host, port, "test-model")
	client.HTTPClient = server.Client()

	err = client.CheckAndPullModel()
	if err != nil {
		t.Errorf("CheckAndPullModel failed when model exists: %v", err)
	}
}

func TestAPIClient_CheckAndPullModel_ModelNotFoundAndPull(t *testing.T) {
	pullRequestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/tags" {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"models": [{"name": "another-model:latest"}]}`))
		} else if r.URL.Path == "/api/pull" {
			if r.Method != http.MethodPost {
				t.Errorf("Expected POST for /api/pull, got %s", r.Method)
			}
			pullRequestReceived = true
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"status": "pulling manifest"}` + "\n"))
			_, _ = w.Write([]byte(`{"status": "verifying sha256", "total": 100, "completed": 50}` + "\n"))
			_, _ = w.Write([]byte(`{"status": "success", "total": 100, "completed": 100}` + "\n"))
		} else {
			http.Error(w, "Not found", http.StatusNotFound)
			t.Fatalf("Unexpected request to %s", r.URL.Path)
		}
	}))
	defer server.Close()

	host, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}

	client := NewAPIClient(host, port, "test-model")
	client.HTTPClient = server.Client()

	originalNewProgram := newProgram
	newProgram = func(model tea.Model, opts ...tea.ProgramOption) *tea.Program {
		// Apply default options first, then test-specific ones
		var finalOpts []tea.ProgramOption
		finalOpts = append(finalOpts, tea.WithoutRenderer())      // Important for CI
		finalOpts = append(finalOpts, tea.WithOutput(io.Discard)) // Suppress output
		finalOpts = append(finalOpts, opts...)

		prog := originalNewProgram(model, finalOpts...)
		go runTeaProgramWithTimeout(prog, t) // Manage lifecycle
		return prog
	}
	defer func() { newProgram = originalNewProgram }()

	err = client.CheckAndPullModel()
	if err != nil {
		t.Errorf("CheckAndPullModel failed during pull: %v", err)
	}
	if !pullRequestReceived {
		t.Error("Pull request was not received by mock server")
	}
}

func TestAPIClient_ProcessMessage(t *testing.T) {
	chatRequestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/api/chat" {
			chatRequestReceived = true
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`{"model": "test-model", "created_at": "2023-01-01T00:00:00Z", "message": {"role": "assistant", "content": "Hello "}}` + "\n"))
			_, _ = w.Write([]byte(`{"model": "test-model", "created_at": "2023-01-01T00:00:01Z", "message": {"role": "assistant", "content": "there!"}, "done": true}` + "\n"))
		} else {
			t.Errorf("Unexpected request: method=%s, path=%s", r.Method, r.URL.Path)
			http.Error(w, "Bad request", http.StatusBadRequest)
		}
	}))
	defer server.Close()

	host, port, err := net.SplitHostPort(strings.TrimPrefix(server.URL, "http://"))
	if err != nil {
		t.Fatalf("Failed to parse server URL: %v", err)
	}

	client := NewAPIClient(host, port, "test-model")
	client.HTTPClient = server.Client()

	session := NewChatSession()
	systemMsg := "You are a test bot."
	userMsg := "Hi"

	rescueStdout := os.Stdout
	rPipe, wPipe, _ := os.Pipe()
	os.Stdout = wPipe
	log.SetOutput(wPipe) // Redirect log output as well for this test

	err = client.ProcessMessage(session, userMsg, systemMsg)

	wPipe.Close()
	// NOP read the pipe to avoid blocking if there's a lot of output
	// go io.Copy(io.Discard, rPipe)
	rPipe.Close()

	os.Stdout = rescueStdout
	log.SetOutput(os.Stderr) // Restore default log output

	if err != nil {
		t.Errorf("ProcessMessage failed: %v", err)
	}
	if !chatRequestReceived {
		t.Error("Chat request was not received by mock server")
	}

	expectedHistory := []Message{
		{Role: "user", Content: "Hi"},
		{Role: "assistant", Content: "Hello there!"},
	}
	actualHistory := session.GetHistory()
	if !reflect.DeepEqual(actualHistory, expectedHistory) {
		t.Errorf("Chat history incorrect. Got: %v, Want: %v", actualHistory, expectedHistory)
	}
}

// NewChatSession is a constructor for ChatSession, ensuring it's initialized.
// If ChatSession is simple (like just a slice), it might not strictly need a constructor
// if direct initialization `&ChatSession{}` is clear enough.
// However, providing one can be good practice for future expansion.
func NewChatSession() *ChatSession {
	return &ChatSession{
		History: make([]Message, 0),
	}
}
