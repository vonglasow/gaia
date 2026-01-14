package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"
)

// Constants for UI styling
const padding = 2

// Styling for help text
var helpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render

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
		m.Completed, m.Total = msg.completed, msg.total
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
	if m.Total > 0 {
		progressPercent = float64(m.Completed) / float64(m.Total)
	}

	pad := strings.Repeat(" ", padding)
	return "\n" +
		pad + m.progress.ViewAs(progressPercent) + "\n\n" +
		pad + helpStyle("Press 'q' to cancel")
}

// ChatHistory stores the conversation history
var chatHistory []Message

// GetChatHistory returns a copy of the current chat history
func GetChatHistory() []Message {
	result := make([]Message, len(chatHistory))
	copy(result, chatHistory)
	return result
}

// SetChatHistory sets the chat history
func SetChatHistory(history []Message) {
	chatHistory = make([]Message, len(history))
	copy(chatHistory, history)
}

// ClearChatHistory clears the chat history
func ClearChatHistory() {
	chatHistory = []Message{}
}

// Main function to process messages and ensure the model exists before sending
func ProcessMessage(msg string) error {
	if strings.TrimSpace(msg) == "" {
		return fmt.Errorf("message cannot be empty")
	}
	useCache := cacheEnabled()
	var cacheKey string
	if useCache {
		key, err := buildCacheKey(msg)
		if err == nil {
			cacheKey = key
			if cached, ok, err := readCache(cacheKey); err == nil && ok {
				fmt.Print(cached)
				fmt.Println()
				chatHistory = append(chatHistory, Message{Role: "user", Content: msg})
				chatHistory = append(chatHistory, Message{Role: "assistant", Content: cached})
				return nil
			}
		}
	}
	if err := checkAndPullIfRequired(); err != nil {
		return err
	}
	response, err := sendMessage(msg)
	if err != nil {
		return err
	}
	if useCache && cacheKey != "" {
		_ = writeCache(cacheKey, response)
	}
	return nil
}

func modelExists(models []tagsModel, modelName string) bool {
	modelName = strings.TrimSpace(modelName)
	if modelName == "" {
		return false
	}

	modelNameHasTag := strings.Contains(modelName, ":")
	modelNameBase := strings.Split(modelName, ":")[0]

	for _, model := range models {
		name := strings.TrimSpace(model.Name)
		if name == "" {
			continue
		}
		if strings.EqualFold(name, modelName) {
			return true
		}
		if !modelNameHasTag {
			modelBase := strings.Split(name, ":")[0]
			if strings.EqualFold(modelBase, modelNameBase) {
				return true
			}
		}
	}

	return false
}

// Function to check if the model exists and pull it if necessary
func checkAndPullIfRequired() error {
	host := viper.GetString("host")
	port := viper.GetInt("port")
	if host == "" {
		return fmt.Errorf("configuration error: host is not set")
	}
	if port <= 0 {
		return fmt.Errorf("configuration error: port is invalid (%d)", port)
	}

	url := fmt.Sprintf("http://%s:%d/api/tags", host, port)

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to connect to API server at %s:%d: %w. Please ensure the server is running", host, port, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("API server returned status %d: %s. Please check server configuration", resp.StatusCode, resp.Status)
	}

	var tagsResponse tagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResponse); err != nil {
		return fmt.Errorf("failed to decode API response: %w. The server may be returning invalid data", err)
	}

	modelName := viper.GetString("model")
	if modelName == "" {
		return fmt.Errorf("configuration error: model name is not set")
	}

	if modelExists(tagsResponse.Models, modelName) {
		return nil
	}

	fmt.Printf("Model %s not found, pulling...\n", modelName)
	return pullModel()
}

// Pull the model using a progress bar
func pullModel() error {
	host := viper.GetString("host")
	port := viper.GetInt("port")
	modelName := viper.GetString("model")
	if modelName == "" {
		return fmt.Errorf("configuration error: model name is not set")
	}

	pullURL := fmt.Sprintf("http://%s:%d/api/pull", host, port)
	pullDataBytes, err := json.Marshal(map[string]string{"name": modelName})
	if err != nil {
		return fmt.Errorf("failed to prepare pull request: %w", err)
	}

	resp, err := http.Post(pullURL, "application/json", bytes.NewBuffer(pullDataBytes))
	if err != nil {
		return fmt.Errorf("failed to connect to API server to pull model '%s': %w. Please ensure the server is running", modelName, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		// Try to read error message from response
		body, _ := io.ReadAll(resp.Body)
		bodyStr := strings.TrimSpace(string(body))
		if bodyStr != "" {
			return fmt.Errorf("failed to pull model '%s': API returned status %d: %s. Response: %s", modelName, resp.StatusCode, resp.Status, bodyStr)
		}
		return fmt.Errorf("failed to pull model '%s': API returned status %d: %s. The model may not exist or the server encountered an error", modelName, resp.StatusCode, resp.Status)
	}

	model := &ProgressModel{progress: progress.New(progress.WithWidth(50))}
	p := tea.NewProgram(model)

	go func() {
		decoder := json.NewDecoder(resp.Body)
		for {
			var pullResponse struct {
				Completed int64 `json:"completed"`
				Total     int64 `json:"total"`
			}
			if err := decoder.Decode(&pullResponse); err != nil {
				if err != io.EOF {
					fmt.Fprintf(os.Stderr, "Warning: error decoding pull progress: %v\n", err)
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

// buildRequestPayload builds the API request payload with system and history messages
func buildRequestPayload(userMessage string) (APIRequest, error) {
	systemRole := viper.GetString("systemrole")
	if systemRole == "" {
		systemRole = viper.GetString("role")
	}
	if systemRole == "" {
		systemRole = "default"
	}

	roleTemplate := viper.GetString(fmt.Sprintf("roles.%s", systemRole))
	systemContent := ""
	if roleTemplate != "" {
		systemContent = fmt.Sprintf(roleTemplate, os.Getenv("SHELL"), runtime.GOOS)
	}

	messages := []Message{{Role: "system", Content: systemContent}}
	messages = append(messages, chatHistory...)
	messages = append(messages, Message{Role: "user", Content: userMessage})

	return APIRequest{
		Model:    viper.GetString("model"),
		Messages: messages,
		Stream:   true,
	}, nil
}

// Send a message to the API
func sendMessage(msg string) (string, error) {
	request, err := buildRequestPayload(msg)
	if err != nil {
		return "", err
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON request: %v", err)
	}

	host := viper.GetString("host")
	port := viper.GetInt("port")
	url := fmt.Sprintf("http://%s:%d/api/chat", host, port)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to connect to API server at %s:%d: %w. Please ensure the server is running", host, port, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API server returned status %d: %s. The request may be invalid or the server is experiencing issues", resp.StatusCode, resp.Status)
	}

	var responseContent string
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
			return "", fmt.Errorf("failed to decode API response: %w. The server may be returning invalid or incomplete data", err)
		}

		if apiResp.Message != nil {
			fmt.Print(apiResp.Message.Content)
			responseContent += apiResp.Message.Content
		}
	}
	fmt.Println()

	// Add user message and assistant response to history
	chatHistory = append(chatHistory, Message{Role: "user", Content: msg})
	chatHistory = append(chatHistory, Message{Role: "assistant", Content: responseContent})

	return responseContent, nil
}

// ProcessMessageWithResponse processes a message and returns the response without printing it
func ProcessMessageWithResponse(msg string) (string, error) {
	if strings.TrimSpace(msg) == "" {
		return "", fmt.Errorf("message cannot be empty")
	}
	useCache := cacheEnabled()
	var cacheKey string
	if useCache {
		key, err := buildCacheKey(msg)
		if err == nil {
			cacheKey = key
			if cached, ok, err := readCache(cacheKey); err == nil && ok {
				chatHistory = append(chatHistory, Message{Role: "user", Content: msg})
				chatHistory = append(chatHistory, Message{Role: "assistant", Content: cached})
				return strings.TrimSpace(cached), nil
			}
		}
	}
	if err := checkAndPullIfRequired(); err != nil {
		return "", err
	}
	response, err := sendMessageWithResponse(msg)
	if err != nil {
		return "", err
	}
	if useCache && cacheKey != "" {
		_ = writeCache(cacheKey, response)
	}
	return response, nil
}

// sendMessageWithResponse sends a message and returns the response without printing
func sendMessageWithResponse(msg string) (string, error) {
	request, err := buildRequestPayload(msg)
	if err != nil {
		return "", err
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return "", fmt.Errorf("failed to marshal JSON request: %v", err)
	}

	host := viper.GetString("host")
	port := viper.GetInt("port")
	url := fmt.Sprintf("http://%s:%d/api/chat", host, port)
	resp, err := http.Post(url, "application/json", bytes.NewBuffer(requestBody))
	if err != nil {
		return "", fmt.Errorf("failed to connect to API server at %s:%d: %w. Please ensure the server is running", host, port, err)
	}
	defer func() {
		if err := resp.Body.Close(); err != nil {
			fmt.Fprintf(os.Stderr, "Warning: failed to close response body: %v\n", err)
		}
	}()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("API server returned status %d: %s. The request may be invalid or the server is experiencing issues", resp.StatusCode, resp.Status)
	}

	var responseContent string
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
			return "", fmt.Errorf("failed to decode API response: %w. The server may be returning invalid or incomplete data", err)
		}

		if apiResp.Message != nil {
			responseContent += apiResp.Message.Content
		}
	}

	// Add user message and assistant response to history
	chatHistory = append(chatHistory, Message{Role: "user", Content: msg})
	chatHistory = append(chatHistory, Message{Role: "assistant", Content: responseContent})

	return strings.TrimSpace(responseContent), nil
}
