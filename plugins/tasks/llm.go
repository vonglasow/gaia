package tasks

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const defaultModel = "qwen2.5:14b"

// TaskMeta holds LLM-inferred metadata for a new or updated task.
type TaskMeta struct {
	Effort        Effort     `json:"effort"`
	Impact        Impact     `json:"impact"`
	Category      Category   `json:"category"`
	Eisenhower    Eisenhower `json:"eisenhower"`
	PriorityScore int        `json:"priority_score"`
}

// PrioritizeResult holds the full prioritization output from the LLM.
type PrioritizeResult struct {
	Tasks     []PrioritizedTask `json:"tasks"`
	Narrative string            `json:"narrative"`
}

// PrioritizedTask holds the LLM score for a single task.
type PrioritizedTask struct {
	ID            string     `json:"id"`
	PriorityScore int        `json:"priority_score"`
	Eisenhower    Eisenhower `json:"eisenhower"`
	Effort        Effort     `json:"effort"`
	Impact        Impact     `json:"impact"`
	Category      Category   `json:"category"`
}

// InferredEntry is one time-tracking entry inferred from a Claude session.
type InferredEntry struct {
	TaskID          string  `json:"task_id"`
	Date            string  `json:"date"`
	DurationMinutes int     `json:"duration_minutes"`
	Confidence      float64 `json:"confidence"`
}

// SessionSummary is the parsed representation of a Claude Code session.
type SessionSummary struct {
	Date            string   `json:"date"`
	DurationMinutes int      `json:"duration_minutes"`
	ProjectPath     string   `json:"project_path"`
	Messages        []string `json:"messages"`
}

// LLMClient calls Ollama for task intelligence operations.
type LLMClient struct {
	baseURL string
	model   string
	timeout time.Duration
	http    *http.Client
}

// NewLLMClient creates an LLMClient pointing at the local Ollama instance.
func NewLLMClient(host string, port int) *LLMClient {
	return newLLMClientFromURL(fmt.Sprintf("http://%s:%d", host, port), 120*time.Second)
}

func newLLMClientFromURL(baseURL string, timeout time.Duration) *LLMClient {
	return &LLMClient{
		baseURL: strings.TrimRight(baseURL, "/"),
		model:   defaultModel,
		timeout: timeout,
		http:    &http.Client{Timeout: timeout},
	}
}

// InferTaskMeta asks the LLM to infer effort, impact, category, and Eisenhower for a new task.
func (c *LLMClient) InferTaskMeta(ctx context.Context, task Task) (TaskMeta, error) {
	prompt := fmt.Sprintf(`You are a task prioritization assistant. Analyze this task and return ONLY valid JSON.

Task: %s - %s
Description: %s

Return JSON with exactly these fields:
{
  "effort": "small"|"medium"|"large",
  "impact": "low"|"medium"|"high",
  "category": "dev"|"management",
  "eisenhower": "Q1"|"Q2"|"Q3"|"Q4",
  "priority_score": 0-100
}

Q1=urgent+important, Q2=important not urgent, Q3=urgent not important, Q4=neither.`,
		task.ID, task.Title, task.Description)

	text, err := c.chat(ctx, prompt)
	if err != nil {
		return TaskMeta{}, err
	}

	var meta TaskMeta
	if err := parseJSON(text, &meta); err != nil {
		return TaskMeta{}, fmt.Errorf("infer meta: %w", err)
	}
	return meta, nil
}

// Prioritize asks the LLM to rank all active tasks and produce a narrative.
func (c *LLMClient) Prioritize(ctx context.Context, tasks []Task, today string) (PrioritizeResult, error) {
	var taskDesc strings.Builder
	for _, t := range tasks {
		fmt.Fprintf(&taskDesc, "- %s: %s [status=%s, priority=%s, deadline=%s]\n",
			t.ID, t.Title, t.Status, t.Priority, t.Deadline)
		if t.Description != "" {
			fmt.Fprintf(&taskDesc, "  %s\n", strings.SplitN(t.Description, "\n", 2)[0])
		}
	}

	prompt := fmt.Sprintf(`You are a task prioritization expert. Today is %s.

Tasks:
%s

Return ONLY valid JSON:
{
  "tasks": [
    {"id":"TXX","priority_score":0-100,"eisenhower":"Q1|Q2|Q3|Q4","effort":"small|medium|large","impact":"low|medium|high","category":"dev|management"}
  ],
  "narrative": "Brief French explanation of top priorities (2-4 sentences)"
}

Include ALL tasks. Sort by priority_score descending.`,
		today, taskDesc.String())

	text, err := c.chat(ctx, prompt)
	if err != nil {
		return PrioritizeResult{}, err
	}

	var result PrioritizeResult
	if err := parseJSON(text, &result); err != nil {
		return PrioritizeResult{}, fmt.Errorf("prioritize: %w", err)
	}
	return result, nil
}

// InferSessions asks the LLM to match Claude sessions to tasks.
func (c *LLMClient) InferSessions(ctx context.Context, tasks []Task, sessions []SessionSummary) ([]InferredEntry, error) {
	var taskList strings.Builder
	for _, t := range tasks {
		fmt.Fprintf(&taskList, "- %s: %s [project=%s]\n", t.ID, t.Title, t.Project)
	}

	var sessionDesc strings.Builder
	for i, s := range sessions {
		fmt.Fprintf(&sessionDesc, "Session %d (%s, %dmin, path=%s):\n", i+1, s.Date, s.DurationMinutes, s.ProjectPath)
		for _, msg := range s.Messages {
			if msg = strings.TrimSpace(msg); msg != "" {
				fmt.Fprintf(&sessionDesc, "  - %s\n", truncate(msg, 200))
			}
		}
	}

	prompt := fmt.Sprintf(`You are a work-log assistant. Given these coding sessions and task list, infer which task(s) were worked on.

Tasks:
%s

Sessions:
%s

Return ONLY valid JSON:
{
  "entries": [
    {"task_id":"TXX","date":"YYYY-MM-DD","duration_minutes":N,"confidence":0.0-1.0}
  ]
}

Only include entries with confidence >= 0.5. Split session time proportionally if multiple tasks.`,
		taskList.String(), sessionDesc.String())

	text, err := c.chat(ctx, prompt)
	if err != nil {
		return nil, err
	}

	var result struct {
		Entries []InferredEntry `json:"entries"`
	}
	if err := parseJSON(text, &result); err != nil {
		return nil, fmt.Errorf("infer sessions: %w", err)
	}
	return result.Entries, nil
}

func (c *LLMClient) chat(ctx context.Context, prompt string) (string, error) {
	payload := map[string]interface{}{
		"model":  c.model,
		"stream": false,
		"messages": []map[string]string{
			{"role": "user", "content": prompt},
		},
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return "", err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/chat", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ollama request: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return "", fmt.Errorf("ollama error: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(b)))
	}

	var decoded struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return "", fmt.Errorf("decoding response: %w", err)
	}
	return decoded.Message.Content, nil
}

// parseJSON extracts the first JSON object from text (handles LLM preamble/postamble).
func parseJSON(text string, v interface{}) error {
	text = strings.TrimSpace(text)
	// Strip markdown code blocks
	if strings.HasPrefix(text, "```") {
		lines := strings.Split(text, "\n")
		if len(lines) > 2 {
			lines = lines[1 : len(lines)-1]
		}
		text = strings.Join(lines, "\n")
	}
	// Find first { or [
	start := strings.IndexAny(text, "{[")
	if start == -1 {
		return fmt.Errorf("no JSON found in LLM response: %q", truncate(text, 200))
	}
	text = text[start:]
	// Find matching end
	end := strings.LastIndexAny(text, "}]")
	if end == -1 {
		return fmt.Errorf("unterminated JSON in LLM response")
	}
	return json.Unmarshal([]byte(text[:end+1]), v)
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}
