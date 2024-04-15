package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"runtime"

	"github.com/spf13/viper"
)

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
	systemrole := "default"

	if viper.IsSet("systemrole") {
		if viper.IsSet(fmt.Sprintf("roles.%s", viper.GetString("systemrole"))) {
			systemrole = viper.GetString("systemrole")
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
