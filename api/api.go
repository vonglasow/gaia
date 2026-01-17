package api

import (
	"fmt"
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

	// Detect role for debug output (even when reading from cache)
	debug := viper.GetBool("debug")
	var detectionResult *DetectionResult
	if viper.GetBool("auto_role.enabled") {
		explicitRole := viper.GetString("systemrole")
		if explicitRole == "" {
			explicitRole = viper.GetString("role")
		}
		if explicitRole != "" {
			detectionResult = &DetectionResult{
				Role:   explicitRole,
				Method: "explicit",
				Reason: "role explicitly provided",
			}
		} else {
			result, err := DetectRole(msg, false) // Don't double-print debug
			if err == nil && result != nil {
				detectionResult = result
			} else {
				detectionResult = &DetectionResult{
					Role:   "default",
					Method: "default",
					Reason: "auto-role detection failed, using default",
				}
			}
		}
	} else {
		explicitRole := viper.GetString("systemrole")
		if explicitRole == "" {
			explicitRole = viper.GetString("role")
		}
		if explicitRole != "" {
			detectionResult = &DetectionResult{
				Role:   explicitRole,
				Method: "explicit",
				Reason: "role explicitly provided",
			}
		} else {
			detectionResult = &DetectionResult{
				Role:   "default",
				Method: "default",
				Reason: "auto-role disabled, using default",
			}
		}
	}

	refreshCache := viper.GetBool("cache.refresh")
	bypassCache := viper.GetBool("cache.bypass")
	useCache := cacheEnabled() || refreshCache
	var cacheKey string
	if useCache {
		key, err := buildCacheKey(msg)
		if err == nil {
			cacheKey = key
			if !bypassCache && !refreshCache {
				if cached, ok, err := readCache(cacheKey); err == nil && ok {
					if debug && detectionResult != nil {
						fmt.Fprintf(os.Stderr, "[DEBUG] Using cached response\n")
						fmt.Fprintf(os.Stderr, "[DEBUG] Role: %s (method: %s", detectionResult.Role, detectionResult.Method)
						if detectionResult.Score > 0 {
							fmt.Fprintf(os.Stderr, ", score: %.2f", detectionResult.Score)
						}
						if detectionResult.Reason != "" {
							fmt.Fprintf(os.Stderr, ", reason: %s", detectionResult.Reason)
						}
						fmt.Fprintf(os.Stderr, ")\n")
					}
					fmt.Print(cached)
					fmt.Println()
					chatHistory = append(chatHistory, Message{Role: "user", Content: msg})
					chatHistory = append(chatHistory, Message{Role: "assistant", Content: cached})
					return nil
				}
			}
		}
	}

	// Display debug info before sending message (if not using cache or cache miss)
	if debug && detectionResult != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] Role: %s (method: %s", detectionResult.Role, detectionResult.Method)
		if detectionResult.Score > 0 {
			fmt.Fprintf(os.Stderr, ", score: %.2f", detectionResult.Score)
		}
		if detectionResult.Reason != "" {
			fmt.Fprintf(os.Stderr, ", reason: %s", detectionResult.Reason)
		}
		fmt.Fprintf(os.Stderr, ")\n")
	}

	if err := checkAndPullIfRequired(); err != nil {
		return err
	}
	response, err := sendMessage(msg)
	if err != nil {
		return err
	}
	if useCache && cacheKey != "" && (!bypassCache || refreshCache) {
		_ = writeCache(cacheKey, response)
	}
	return nil
}

// buildRequestPayload builds the API request payload with system and history messages
func buildRequestPayload(userMessage string) (APIRequest, error) {
	systemRole := viper.GetString("systemrole")
	if systemRole == "" {
		systemRole = viper.GetString("role")
	}

	// Auto-detect role if not explicitly set and auto-role is enabled
	if systemRole == "" && viper.GetBool("auto_role.enabled") {
		debug := viper.GetBool("debug")
		detectionResult, err := DetectRole(userMessage, debug)
		if err == nil && detectionResult != nil {
			systemRole = detectionResult.Role
		} else {
			systemRole = "default"
		}
	}

	if systemRole == "" {
		systemRole = "default"
	}

	roleTemplate := viper.GetString(fmt.Sprintf("roles.%s", systemRole))
	systemContent := ""
	if roleTemplate != "" {
		systemContent = fmt.Sprintf(roleTemplate, os.Getenv("SHELL"), runtime.GOOS)
	}

	// Pre-allocate with exact capacity: system message + chat history + user message
	messages := make([]Message, 0, len(chatHistory)+2)
	messages = append(messages, Message{Role: "system", Content: systemContent})
	messages = append(messages, chatHistory...)
	messages = append(messages, Message{Role: "user", Content: userMessage})

	return APIRequest{
		Model:    viper.GetString("model"),
		Messages: messages,
		Stream:   true,
	}, nil
}

// ProcessMessageWithResponse processes a message and returns the response without printing it
func ProcessMessageWithResponse(msg string) (string, error) {
	if strings.TrimSpace(msg) == "" {
		return "", fmt.Errorf("message cannot be empty")
	}

	// Detect role for debug output (even when reading from cache)
	debug := viper.GetBool("debug")
	var detectionResult *DetectionResult
	if viper.GetBool("auto_role.enabled") {
		explicitRole := viper.GetString("systemrole")
		if explicitRole == "" {
			explicitRole = viper.GetString("role")
		}
		if explicitRole != "" {
			detectionResult = &DetectionResult{
				Role:   explicitRole,
				Method: "explicit",
				Reason: "role explicitly provided",
			}
		} else {
			result, err := DetectRole(msg, false) // Don't double-print debug
			if err == nil && result != nil {
				detectionResult = result
			} else {
				detectionResult = &DetectionResult{
					Role:   "default",
					Method: "default",
					Reason: "auto-role detection failed, using default",
				}
			}
		}
	} else {
		explicitRole := viper.GetString("systemrole")
		if explicitRole == "" {
			explicitRole = viper.GetString("role")
		}
		if explicitRole != "" {
			detectionResult = &DetectionResult{
				Role:   explicitRole,
				Method: "explicit",
				Reason: "role explicitly provided",
			}
		} else {
			detectionResult = &DetectionResult{
				Role:   "default",
				Method: "default",
				Reason: "auto-role disabled, using default",
			}
		}
	}

	refreshCache := viper.GetBool("cache.refresh")
	bypassCache := viper.GetBool("cache.bypass")
	useCache := cacheEnabled() || refreshCache
	var cacheKey string
	if useCache {
		key, err := buildCacheKey(msg)
		if err == nil {
			cacheKey = key
			if !bypassCache && !refreshCache {
				if cached, ok, err := readCache(cacheKey); err == nil && ok {
					if debug && detectionResult != nil {
						fmt.Fprintf(os.Stderr, "[DEBUG] Using cached response\n")
						fmt.Fprintf(os.Stderr, "[DEBUG] Role: %s (method: %s", detectionResult.Role, detectionResult.Method)
						if detectionResult.Score > 0 {
							fmt.Fprintf(os.Stderr, ", score: %.2f", detectionResult.Score)
						}
						if detectionResult.Reason != "" {
							fmt.Fprintf(os.Stderr, ", reason: %s", detectionResult.Reason)
						}
						fmt.Fprintf(os.Stderr, ")\n")
					}
					chatHistory = append(chatHistory, Message{Role: "user", Content: msg})
					chatHistory = append(chatHistory, Message{Role: "assistant", Content: cached})
					return strings.TrimSpace(cached), nil
				}
			}
		}
	}

	// Display debug info before sending message (if not using cache or cache miss)
	if debug && detectionResult != nil {
		fmt.Fprintf(os.Stderr, "[DEBUG] Role: %s (method: %s", detectionResult.Role, detectionResult.Method)
		if detectionResult.Score > 0 {
			fmt.Fprintf(os.Stderr, ", score: %.2f", detectionResult.Score)
		}
		if detectionResult.Reason != "" {
			fmt.Fprintf(os.Stderr, ", reason: %s", detectionResult.Reason)
		}
		fmt.Fprintf(os.Stderr, ")\n")
	}

	if err := checkAndPullIfRequired(); err != nil {
		return "", err
	}
	response, err := sendMessageInternal(msg, false)
	if err != nil {
		return "", err
	}
	if useCache && cacheKey != "" && (!bypassCache || refreshCache) {
		_ = writeCache(cacheKey, response)
	}
	return strings.TrimSpace(response), nil
}
