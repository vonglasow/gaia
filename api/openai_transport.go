package api

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/spf13/viper"
)

type openaiRoundTripper struct {
	base http.RoundTripper
}

type openAIChatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type tagsResponse struct {
	Models []struct {
		Name string `json:"name"`
	} `json:"models"`
}

func init() {
	base := http.DefaultTransport
	http.DefaultTransport = &openaiRoundTripper{base: base}

	if http.DefaultClient != nil {
		http.DefaultClient.Transport = http.DefaultTransport
	}
}

func (rt *openaiRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	if !strings.EqualFold(req.URL.Hostname(), "api.openai.com") {
		return rt.base.RoundTrip(req)
	}

	hostCfg := strings.TrimSpace(viper.GetString("host"))
	portCfg := viper.GetInt("port")

	if !strings.Contains(hostCfg, "api.openai.com") || portCfg != 443 {
		return rt.base.RoundTrip(req)
	}

	switch req.URL.Path {
	case "/api/tags":
		return rt.handleTags(req)
	case "/api/pull":
		return rt.handlePull(req)
	case "/api/chat":
		return rt.handleChat(req)
	default:
		return rt.base.RoundTrip(req)
	}
}

func (rt *openaiRoundTripper) handleTags(req *http.Request) (*http.Response, error) {
	modelName := viper.GetString("model")
	if modelName == "" {
		modelName = "gpt-4o-mini"
	}

	var resp tagsResponse
	resp.Models = append(resp.Models, struct {
		Name string `json:"name"`
	}{Name: modelName})

	bodyBytes, err := json.Marshal(resp)
	if err != nil {
		return nil, err
	}

	body := io.NopCloser(bytes.NewReader(bodyBytes))

	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Header:        make(http.Header),
		Body:          body,
		ContentLength: int64(len(bodyBytes)),
		Request:       req,
	}, nil
}

func (rt *openaiRoundTripper) handlePull(req *http.Request) (*http.Response, error) {
	body := io.NopCloser(bytes.NewReader([]byte{}))

	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Header:        make(http.Header),
		Body:          body,
		ContentLength: 0,
		Request:       req,
	}, nil
}

func (rt *openaiRoundTripper) handleChat(req *http.Request) (*http.Response, error) {
	apiKey := os.Getenv("OPENAI_API_KEY")
	if apiKey == "" {
		return nil, fmt.Errorf("OPENAI_API_KEY is not set")
	}

	bodyBytes, err := io.ReadAll(req.Body)
	if err != nil {
		return nil, fmt.Errorf("read request body: %w", err)
	}
	_ = req.Body.Close()

	var gaiaReq APIRequest
	if err := json.Unmarshal(bodyBytes, &gaiaReq); err != nil {
		return nil, fmt.Errorf("decode APIRequest: %w", err)
	}

	modelName := gaiaReq.Model
	if modelName == "" {
		modelName = viper.GetString("model")
		if modelName == "" {
			modelName = "gpt-4o-mini"
		}
	}

	openaiPayload := struct {
		Model    string    `json:"model"`
		Messages []Message `json:"messages"`
	}{
		Model:    modelName,
		Messages: gaiaReq.Messages,
	}

	payloadBytes, err := json.Marshal(openaiPayload)
	if err != nil {
		return nil, fmt.Errorf("marshal OpenAI payload: %w", err)
	}

	openaiReq, err := http.NewRequest(
		http.MethodPost,
		"https://api.openai.com/v1/chat/completions",
		bytes.NewReader(payloadBytes),
	)
	if err != nil {
		return nil, fmt.Errorf("build OpenAI request: %w", err)
	}

	openaiReq.Header.Set("Content-Type", "application/json")
	openaiReq.Header.Set("Authorization", "Bearer "+apiKey)

	client := &http.Client{Transport: rt.base}
	openaiResp, err := client.Do(openaiReq)
	if err != nil {
		return nil, fmt.Errorf("call OpenAI: %w", err)
	}
	defer func() {
		if err := openaiResp.Body.Close(); err != nil {
			fmt.Printf("warning: failed to close OpenAI response body: %v\n", err)
		}
	}()

	if openaiResp.StatusCode < 200 || openaiResp.StatusCode >= 300 {
		errBody, _ := io.ReadAll(openaiResp.Body)
		return nil, fmt.Errorf("OpenAI error: %s - %s", openaiResp.Status, string(errBody))
	}

	respBytes, err := io.ReadAll(openaiResp.Body)
	if err != nil {
		return nil, fmt.Errorf("read OpenAI response: %w", err)
	}

	var oaResp openAIChatCompletionResponse
	if err := json.Unmarshal(respBytes, &oaResp); err != nil {
		return nil, fmt.Errorf("decode OpenAI response: %w", err)
	}

	if len(oaResp.Choices) == 0 {
		return nil, fmt.Errorf("OpenAI response has no choices")
	}

	content := oaResp.Choices[0].Message.Content

	apiResp := APIResponse{
		Model: modelName,
		Message: &Message{
			Role:    "assistant",
			Content: content,
		},
	}

	apiRespBytes, err := json.Marshal(apiResp)
	if err != nil {
		return nil, fmt.Errorf("marshal APIResponse: %w", err)
	}

	body := io.NopCloser(bytes.NewReader(apiRespBytes))

	return &http.Response{
		StatusCode:    http.StatusOK,
		Status:        "200 OK",
		Header:        http.Header{"Content-Type": []string{"application/json"}},
		Body:          body,
		ContentLength: int64(len(apiRespBytes)),
		Request:       req,
	}, nil
}
