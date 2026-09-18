package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"gaia/plugins/shared"
	"io"
	"net/http"
	"strings"
	"sync"
)

type OllamaProvider struct{}

func NewOllamaProvider() *OllamaProvider { return &OllamaProvider{} }

func (p *OllamaProvider) Name() string { return "ollama" }

func (p *OllamaProvider) Send(ctx context.Context, req AskRequest) (AskResponse, error) {
	if err := p.ensureModel(ctx, req); err != nil {
		return AskResponse{}, err
	}
	reqCtx, cancel := withTimeout(ctx, req.Timeout)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d/api/chat", req.Host, req.Port)
	payload := map[string]any{
		"model":  req.Model,
		"stream": false,
		// buildToolMessages keeps calls and tool names structured, not flattened.
		"messages": buildToolMessages(req),
	}
	if keepAlive := KeepAlive(req); keepAlive != "" {
		payload["keep_alive"] = keepAlive
	}
	if tools := toOllamaTools(req.Tools); len(tools) > 0 {
		payload["tools"] = tools
	}
	payload["options"] = map[string]any{"num_ctx": contextWindow(req)}
	body, err := json.Marshal(payload)
	if err != nil {
		return AskResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return AskResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: req.Timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return AskResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return AskResponse{}, fmt.Errorf("ollama error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var decoded struct {
		Message struct {
			Content   string           `json:"content"`
			ToolCalls []ollamaToolCall `json:"tool_calls"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return AskResponse{}, err
	}
	return AskResponse{
		Text:      decoded.Message.Content,
		ToolCalls: parseToolCalls(decoded.Message.ToolCalls),
	}, nil
}

func (p *OllamaProvider) SendStream(ctx context.Context, req AskRequest, onChunk func(string)) (AskResponse, error) {
	if err := p.ensureModel(ctx, req); err != nil {
		return AskResponse{}, err
	}
	reqCtx, cancel := withTimeout(ctx, req.Timeout)
	defer cancel()
	url := fmt.Sprintf("http://%s:%d/api/chat", req.Host, req.Port)
	payload := map[string]any{
		"model":    req.Model,
		"stream":   true,
		"messages": buildMessages(req),
	}
	if keepAlive := KeepAlive(req); keepAlive != "" {
		payload["keep_alive"] = keepAlive
	}
	payload["options"] = map[string]any{"num_ctx": contextWindow(req)}
	body, err := json.Marshal(payload)
	if err != nil {
		return AskResponse{}, err
	}
	httpReq, err := http.NewRequestWithContext(reqCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return AskResponse{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: req.Timeout}
	resp, err := client.Do(httpReq)
	if err != nil {
		return AskResponse{}, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return AskResponse{}, fmt.Errorf("ollama error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var full strings.Builder
	decoder := json.NewDecoder(resp.Body)
	for {
		var chunk struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
			Done bool `json:"done"`
		}
		if err := decoder.Decode(&chunk); err != nil {
			if err == io.EOF {
				break
			}
			return AskResponse{}, err
		}
		if chunk.Message.Content != "" {
			onChunk(chunk.Message.Content)
			full.WriteString(chunk.Message.Content)
		}
		if chunk.Done {
			break
		}
	}
	return AskResponse{Text: full.String()}, nil
}

func (p *OllamaProvider) ensureModel(ctx context.Context, req AskRequest) error {
	model := strings.TrimSpace(req.Model)
	if model == "" {
		return nil
	}
	client := &http.Client{Timeout: req.Timeout}
	baseURL := fmt.Sprintf("http://%s:%d", req.Host, req.Port)
	exists, err := p.modelExists(ctx, client, baseURL, model)
	if err != nil {
		return err
	}
	if exists && !req.Pull {
		return nil
	}
	return p.pullModel(ctx, client, baseURL, model, req.ProgressOut, req.ProgressClearer)
}

func (p *OllamaProvider) modelExists(ctx context.Context, client *http.Client, baseURL, model string) (bool, error) {
	url := fmt.Sprintf("%s/api/tags", baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false, err
	}
	resp, err := client.Do(httpReq)
	if err != nil {
		return false, err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return false, fmt.Errorf("ollama tags error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var decoded struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return false, err
	}
	model = strings.TrimSpace(model)
	if model == "" {
		return false, nil
	}
	hasTag := strings.Contains(model, ":")
	for _, entry := range decoded.Models {
		name := strings.TrimSpace(entry.Name)
		if name == "" {
			continue
		}
		if hasTag {
			if name == model {
				return true, nil
			}
			continue
		}
		if name == model || strings.HasPrefix(name, model+":") {
			return true, nil
		}
	}
	return false, nil
}

func (p *OllamaProvider) pullModel(_ context.Context, client *http.Client, baseURL, model string, out io.Writer, clearer *shared.ProgressClearer) error {
	pullCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	url := fmt.Sprintf("%s/api/pull", baseURL)
	payload := map[string]any{
		"name":   model,
		"stream": true,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	httpReq, err := http.NewRequestWithContext(pullCtx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	resp, err := client.Do(httpReq)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("ollama pull error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	type pullEvent struct {
		Status    string `json:"status"`
		Completed int64  `json:"completed"`
		Total     int64  `json:"total"`
		Error     string `json:"error"`
		Done      bool   `json:"done"`
	}
	if out == nil {
		out = io.Discard
	}

	progressModel := shared.NewProgressModel(50)
	cancelled := false
	var closeOnce sync.Once
	progressModel.OnCancel = func() {
		cancelled = true
		cancel()
		closeOnce.Do(func() {
			_ = resp.Body.Close()
		})
	}
	prg := shared.NewProgressProgram(progressModel, out)
	errCh := make(chan error, 1)

	go func() {
		defer func() {
			prg.Send(shared.ProgressDone{})
			close(errCh)
		}()
		decoder := json.NewDecoder(resp.Body)
		for {
			var event pullEvent
			if err := decoder.Decode(&event); err != nil {
				if err == io.EOF {
					return
				}
				if pullCtx.Err() != nil {
					return
				}
				errCh <- err
				return
			}
			if strings.TrimSpace(event.Error) != "" {
				errCh <- fmt.Errorf("ollama pull error: %s", strings.TrimSpace(event.Error))
				return
			}
			if event.Completed > 0 || event.Total > 0 {
				prg.Send(shared.ProgressUpdate{Completed: event.Completed, Total: event.Total})
			}
			if event.Done {
				return
			}
		}
	}()

	if _, err := prg.Run(); err != nil {
		return fmt.Errorf("running the progress UI: %w", err)
	}
	if cancelled {
		return fmt.Errorf("ollama pull canceled")
	}
	if clearer != nil {
		clearer.MarkPending()
	}
	if err, ok := <-errCh; ok && err != nil {
		return err
	}
	return nil
}
