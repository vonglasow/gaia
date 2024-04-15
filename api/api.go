package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"os"
	"runtime"
	"strings"

	"github.com/spf13/viper"
)

func roundFloat(val float64, precision uint) float64 {
	ratio := math.Pow(10, float64(precision))
	return math.Round(val*ratio) / ratio
}

type Message struct {
	Content string `json:"content"`
	Role    string `json:"role"`
}

type APIRequest struct {
	Model    string    `json:"model"`
	Messages []Message `json:"messages"`
	Stream   bool      `json:"stream"`
}

type APIResponse struct {
	Model    string   `json:"model"`
	Response string   `json:"response"`
	Message  *Message `json:"message"`
}

func processStreamedResponse(body io.Reader) {
	decoder := json.NewDecoder(body)
	respChan := make(chan string)
	eofChan := make(chan struct{})
	errorChan := make(chan error)

	go func() {
		var apiResp APIResponse
		for {
			if err := decoder.Decode(&apiResp); err == io.EOF {
				eofChan <- struct{}{}
			} else if err != nil {
				errorChan <- err
			} else {
				respChan <- apiResp.Message.Content
			}
		}
	}()

	for {
		select {
		case resp := <-respChan:
			fmt.Print(resp)
		case err := <-errorChan:
			fmt.Println("Error decoding JSON:", err)
			return
		case <-eofChan:
			fmt.Println()
			return
		}
	}
}

func ProcessMessage(msg string) error {
	if err := checkAndPullIfRequired(); err != nil {
		return err
	}

	systemrole := "default"

	if viper.IsSet("systemrole") {
		if viper.IsSet(fmt.Sprintf("roles.%s", viper.GetString("systemrole"))) {
			systemrole = viper.GetString("systemrole")
		} else if viper.GetString("systemrole") == "" {
			systemrole = "default"
		} else {
			fmt.Printf("Error: Role '%s' not found in the configuration", viper.GetString("systemrole"))
			return nil
		}
	}

	role := fmt.Sprintf(viper.GetString(fmt.Sprintf("roles.%s", systemrole)), os.Getenv("SHELL"), runtime.GOOS)

	request := APIRequest{
		Model: viper.GetString("model"),
		Messages: []Message{
			{
				Role:    "system",
				Content: role,
			},
			{
				Role:    "user",
				Content: msg,
			},
		},
		Stream: true,
	}

	requestBody, err := json.Marshal(request)
	if err != nil {
		fmt.Println("Error during call on API")
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

type Model struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type TagsResponse struct {
	Models []Model `json:"models"`
}

type pulling struct {
	Status    string `json:"status"`
	Digest    string `json:"digest,omitempty"`
	Total     int64  `json:"total,omitempty"`
	Completed int64  `json:"completed,omitempty"`
}

func checkAndPullIfRequired() error {
	url := fmt.Sprintf("http://%s:%d/api/tags", viper.GetString("host"), viper.GetInt("port"))

	resp, err := http.Get(url)
	if err != nil {
		return fmt.Errorf("failed to fetch tags from %s: %v", url, err)
	}
	defer resp.Body.Close()

	var tagsResponse TagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&tagsResponse); err != nil {
		return fmt.Errorf("failed to decode tags response: %v", err)
	}

	// Check if OLLAMA_MODEL exists in the models list
	modelExists := false
	for _, model := range tagsResponse.Models {
		extractedBaseModel := strings.Split(model.Name, ":")[0]
		if extractedBaseModel == viper.GetString("model") {
			modelExists = true
			break
		}
	}

	if !modelExists {
		fmt.Printf("Model %s not found in the tags.\n", viper.GetString("model"))
		fmt.Printf("Pulling model %s.\n", viper.GetString("model"))

		pullURL := fmt.Sprintf("http://%s:%d/api/pull", viper.GetString("host"), viper.GetInt("port"))
		pullData := map[string]string{"name": viper.GetString("model")}
		pullDataBytes, _ := json.Marshal(pullData)

		// Make POST request to initiate the pull operation
		contentType := "application/json"
		resp, err := http.Post(pullURL, contentType, bytes.NewBuffer(pullDataBytes))
		if err != nil {
			return fmt.Errorf("failed to initiate pull for model %s: %v", viper.GetString("model"), err)
		}
		defer resp.Body.Close()

		// Check response status code
		if resp.StatusCode != http.StatusOK {
			return fmt.Errorf("pull request failed with status code: %d", resp.StatusCode)
		}

		// Start processing the streamed response using channels
		processPullStreamedResponse(resp.Body)
	}

	return nil
}

// Function to process the streamed response using channels
func processPullStreamedResponse(body io.Reader) {
	decoder := json.NewDecoder(body)
	statusChan := make(chan pulling)
	errorChan := make(chan error)
	doneChan := make(chan struct{})

	go func() {
		for {
			var pullResponse pulling
			if err := decoder.Decode(&pullResponse); err == io.EOF {
				close(doneChan)
				return
			} else if err != nil {
				errorChan <- err
				return
			}
			statusChan <- pullResponse
		}
	}()

	for {
		select {
		case status := <-statusChan:
			if strings.Split(status.Status, " ")[0] == "pulling" {
				percent := float64(status.Completed) / float64(status.Total)
				if math.IsNaN(percent) {
					percent = 0
				}
				fmt.Println("Download in progress:", roundFloat(percent*100, 2))
			}
			if status.Status == "success" {
				fmt.Println("Image successfully pulled.")
				return
			}
		case err := <-errorChan:
			fmt.Println("Error decoding JSON:", err)
			return
		case <-doneChan:
			fmt.Println("Streamed response processing completed.")
			return
		}
	}
}
