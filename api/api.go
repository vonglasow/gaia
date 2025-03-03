package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/spf13/viper"
)

// Constants for UI styling
const (
	padding  = 2
	maxWidth = 80
)

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

// Process the streamed response from the API
func processStreamedResponse(body io.ReadCloser) {
	defer body.Close()
	decoder := json.NewDecoder(body)

	for {
		var apiResp APIResponse
		if err := decoder.Decode(&apiResp); err != nil {
			if err == io.EOF {
				fmt.Println()
				return
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				return
			}
			fmt.Println("Error decoding JSON:", err)
			return
		}

		if apiResp.Message != nil {
			fmt.Print(apiResp.Message.Content)
		}
	}
}

// Main function to process messages and ensure the model exists before sending
func ProcessMessage(msg string) error {
	if err := checkAndPullIfRequired(); err != nil {
		return err
	}

	return sendMessage(msg)
}

// Function to check if the model exists and pull it if necessary
func checkAndPullIfRequired() error {
	url := fmt.Sprintf("http://%s:%d/api/tags", viper.GetString("host"), viper.GetInt("port"))

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch tags: %v", err)
	}
	defer resp.Body.Close()

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
		if strings.Split(model.Name, ":")[0] == viper.GetString("model") {
			modelExists = true
			break
		}
	}

	if !modelExists {
		fmt.Printf("Model %s not found, pulling...\n", viper.GetString("model"))
		return pullModel()
	}

	return nil
}

// Pull the model using a progress bar
func pullModel() error {
	pullURL := fmt.Sprintf("http://%s:%d/api/pull", viper.GetString("host"), viper.GetInt("port"))
	pullData := map[string]string{"name": viper.GetString("model")}
	pullDataBytes, _ := json.Marshal(pullData)

	resp, err := http.Post(pullURL, "application/json", bytes.NewBuffer(pullDataBytes))
	if err != nil {
		return fmt.Errorf("failed to pull model: %v", err)
	}
	defer resp.Body.Close()

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

// Send a message to the API
func sendMessage(msg string) error {
	request := APIRequest{
		Model: viper.GetString("model"),
		Messages: []Message{
			{Role: "system", Content: "System message"},
			{Role: "user", Content: msg},
		},
		Stream: true,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal JSON request: %v", err)
	}

	url := fmt.Sprintf("http://%s:%d/api/chat", viper.GetString("host"), viper.GetInt("port"))
	contentType := "application/json"

	resp, err := http.Post(url, contentType, bytes.NewBuffer(requestBody))
	if err != nil {
		return fmt.Errorf("failed to make HTTP request: %v", err)
	}
	defer resp.Body.Close()

	processStreamedResponse(resp.Body)
	return nil
}
