package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	// "github.com/spf13/viper" // Removed
	// "os" // Removed
	"time"
)

// Allow overriding tea.NewProgram for tests
var newProgram = func(model tea.Model, opts ...tea.ProgramOption) *tea.Program {
	return tea.NewProgram(model, opts...)
}

// Constants for UI styling
const (
	padding  = 2
	maxWidth = 80
)

// Styling for help text
var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render

// APIClient manages interactions with the API
type APIClient struct {
	BaseURL    string
	HTTPClient *http.Client
	ModelName  string
}

// NewAPIClient creates a new API client
func NewAPIClient(host, port, modelName string) *APIClient {
	return &APIClient{
		BaseURL: fmt.Sprintf("http://%s:%s", host, port), // BaseURL does not include /api
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second, // 30-second timeout
		},
		ModelName: modelName,
	}
}

// getURL constructs the full URL for a given API endpoint path
func (c *APIClient) getURL(endpointPath string) string {
	// Ensure endpointPath starts with a "/"
	if !strings.HasPrefix(endpointPath, "/") {
		endpointPath = "/" + endpointPath
	}
	return c.BaseURL + "/api" + endpointPath // Adds /api segment
}

// doGetRequest performs a GET request and returns the response
func (c *APIClient) doGetRequest(url string) (*http.Response, error) {
	resp, err := c.HTTPClient.Get(url)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// doPostRequest performs a POST request and returns the response
func (c *APIClient) doPostRequest(url string, body []byte) (*http.Response, error) {
	resp, err := c.HTTPClient.Post(url, "application/json", bytes.NewBuffer(body))
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// ChatSession stores the conversation history for a session
type ChatSession struct {
	History []Message
}

// NewChatSession creates a new, empty chat session.
func NewChatSession() *ChatSession {
	return &ChatSession{
		History: make([]Message, 0), // Initialize history slice
	}
}

// AddMessage appends a message to the chat history
func (cs *ChatSession) AddMessage(role, content string) {
	cs.History = append(cs.History, Message{Role: role, Content: content})
}

// GetHistory returns the chat history
func (cs *ChatSession) GetHistory() []Message {
	return cs.History
}

// Message structure for API interactions
type Message struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

// API request structure
type APIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

// API response structure
type APIResponse struct {
	Model    string   `json:"model"`
	Response string   `json:"response"`
	Message  *Message `json:"message"`
}

// ProgressModel manages the download progress
type ProgressModel struct {
	progress  progress.Model
	Total     int64
	Completed int64
	Done      bool
	mutex     sync.Mutex
}

func (m *ProgressModel) Init() tea.Cmd {
	return nil
}

// Update the progress model with new data
func (m *ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case string:
		if msg == "done" {
			m.Done = true
			return m, tea.Quit
		}
	case tea.KeyMsg:
		if msg.String() == "q" {
			return m, tea.Quit
		}
	case struct {
		completed int64
		total     int64
	}:
		m.mutex.Lock()
		m.Completed = msg.completed
		m.Total = msg.total
		m.mutex.Unlock()
		m.progress.SetPercent(float64(m.Completed) / float64(m.Total))
	}

	return m, nil
}

// View function to render the progress bar
func (m *ProgressModel) View() string {
	if m.Done {
		return "\nDownload completed! Proceeding...\n"
	}

	progressPercent := float64(0)
	if m.Total != 0 {
		progressPercent = float64(m.Completed) / float64(m.Total)
	}

	pad := strings.Repeat(" ", padding)
	return "\n" +
		pad + m.progress.ViewAs(progressPercent) + "\n\n" +
		pad + helpStyle("Press 'q' to cancel")
}

// ProcessMessage processes the user message, manages history, and sends it to the API
func (c *APIClient) ProcessMessage(session *ChatSession, userMessage string, systemMessage string) error {
	if err := c.CheckAndPullModel(); err != nil {
		return err
	}

	session.AddMessage("user", userMessage)

	return c.sendMessage(session, systemMessage)
}

// CheckAndPullModel checks if the model exists and pulls it if necessary
func (c *APIClient) CheckAndPullModel() error {
	url := c.getURL("/tags") // Changed: path should be relative to /api

	resp, err := c.doGetRequest(url)
	if err != nil {
		return fmt.Errorf("failed to fetch tags: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	var tagsResponse struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&tagsResponse); err != nil {
		return fmt.Errorf("failed to decode tags response: %v", err)
	}

	modelExists := false
	for _, model := range tagsResponse.Models {
		if strings.Split(model.Name, ":")[0] == c.ModelName {
			modelExists = true
			break
		}
	}

	if !modelExists {
		log.Printf("Model %s not found, pulling...\n", c.ModelName) // Changed from fmt.Printf
		return c.pullModel()
	}

	return nil
}

// pullModel pulls the model using a progress bar
func (c *APIClient) pullModel() error {
	pullURL := c.getURL("/pull") // Changed: path should be relative to /api
	pullData := map[string]string{"name": c.ModelName}
	pullDataBytes, err := json.Marshal(pullData)
	if err != nil {
		return fmt.Errorf("failed to marshal pull data: %v", err)
	}

	resp, err := c.doPostRequest(pullURL, pullDataBytes)
	if err != nil {
		return fmt.Errorf("failed to pull model: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	// Ensure progress.WithWidth is a valid option if used, or use a default.
	// For example, progress.WithDefaultGradient() is a common one.
	// The test specified WithWidth(50), let's see if that's okay or if we should use a more standard default.
	// Let's assume WithWidth is fine, if not, tests might fail and we can adjust.
	// The critical part is `newProgram` call.
	model := &ProgressModel{progress: progress.New(progress.WithWidth(50))} // Default from original code
	p := newProgram(model)                                                  // Changed tea.NewProgram to newProgram

	go func() {
		decoder := json.NewDecoder(resp.Body)
		for {
			var pullResponse struct {
				Completed int64 `json:"completed"`
				Total     int64 `json:"total"`
			}
			if err := decoder.Decode(&pullResponse); err != nil {
				if err != io.EOF && !strings.Contains(err.Error(), "use of closed network connection") {
					log.Printf("Error decoding pull progress: %v", err)
				}
				break
			}
			p.Send(struct {
				completed int64
				total     int64
			}{pullResponse.Completed, pullResponse.Total})
		}
		p.Send("done")
	}()

	if _, err := p.Run(); err != nil {
		return fmt.Errorf("error running progress UI: %v", err)
	}

	return nil
}

// sendMessage sends the accumulated messages to the API
func (c *APIClient) sendMessage(session *ChatSession, systemMessage string) error {
	messages := make([]Message, 0)
	if systemMessage != "" {
		messages = append(messages, Message{
			Role:    "system",
			Content: systemMessage,
		})
	}
	messages = append(messages, session.GetHistory()...)

	request := APIRequest{
		Model:    c.ModelName,
		Messages: messages,
		Stream:   true,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON request: %v", err)
	}

	url := c.getURL("/chat") // Changed: path should be relative to /api

	resp, err := c.doPostRequest(url, requestBody)
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %v", err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			log.Printf("Error closing response body: %v", err)
		}
	}()

	var responseContent strings.Builder
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
			return fmt.Errorf("error decoding JSON: %v", err)
		}

		if apiResp.Message != nil {
			fmt.Print(apiResp.Message.Content)
			responseContent.WriteString(apiResp.Message.Content)
		}
	}
	fmt.Println()

	session.AddMessage("assistant", responseContent.String())

	return nil
}
