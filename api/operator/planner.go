package operator

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"

	"gaia/api"
)

// State holds the goal and conversation steps (assistant decision + user observation) for the operator loop.
type State struct {
	Goal  string
	Steps []Step
}

// Step represents one turn: either assistant (decision) or user (observation).
type Step struct {
	Role    string // "assistant" or "user"
	Content string
}

// Decision is the parsed LLM output for one turn: either answer (done) or tool call.
type Decision struct {
	Action    string            `json:"action"`    // "answer" or "tool"
	Content   string            `json:"content"`   // for answer
	Name      string            `json:"name"`      // for tool
	Args      map[string]string `json:"args"`      // for tool
	Reasoning string            `json:"reasoning"` // optional, for debug only
}

// AppendObservation adds a user message (tool result or error) to state.
func (s *State) AppendObservation(content string) {
	s.Steps = append(s.Steps, Step{Role: "user", Content: content})
}

// AppendDecision adds an assistant message (the raw JSON decision) to state.
// Only the JSON is stored; reasoning is not re-fed to the model.
func (s *State) AppendDecision(raw string) {
	s.Steps = append(s.Steps, Step{Role: "assistant", Content: raw})
}

// LastAnswerOrPartial returns the last assistant content if any, else the goal.
func (s *State) LastAnswerOrPartial() string {
	for i := len(s.Steps) - 1; i >= 0; i-- {
		if s.Steps[i].Role == "assistant" {
			return s.Steps[i].Content
		}
	}
	return s.Goal
}

// Planner builds the prompt and calls the LLM to get the next decision.
type Planner struct {
	Model   string
	SendReq func(api.APIRequest) (string, error)
}

// Decide builds messages from state + registry (tools list), sends to LLM, parses JSON into Decision.
func (p *Planner) Decide(ctx context.Context, state *State, registry *Registry) (*Decision, string, error) {
	messages := p.buildMessages(state, registry)
	model := p.Model
	if model == "" {
		model = "default"
	}
	req := api.APIRequest{
		Model:    model,
		Messages: messages,
		Stream:   false,
	}
	sendReq := p.SendReq
	if sendReq == nil {
		sendReq = api.SendRequestNoStream
	}
	raw, err := sendReq(req)
	if err != nil {
		return nil, "", err
	}
	raw = extractJSON(raw)
	var dec Decision
	if err := json.Unmarshal([]byte(raw), &dec); err != nil {
		return nil, raw, fmt.Errorf("invalid JSON: %w", err)
	}
	if dec.Action != "answer" && dec.Action != "tool" {
		return nil, raw, fmt.Errorf("invalid action %q (expected answer or tool)", dec.Action)
	}
	if dec.Action == "tool" && (dec.Name == "" || dec.Args == nil) {
		return nil, raw, fmt.Errorf("tool decision missing name or args")
	}
	return &dec, raw, nil
}

var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*([^`]+)```")

// extractJSON returns the first JSON object from s, optionally inside ```json ... ```.
func extractJSON(s string) string {
	s = strings.TrimSpace(s)
	if m := jsonBlockRe.FindStringSubmatch(s); len(m) >= 2 {
		return strings.TrimSpace(m[1])
	}
	// Try to find a single {...} block
	start := strings.Index(s, "{")
	if start < 0 {
		return s
	}
	depth := 0
	for i := start; i < len(s); i++ {
		switch s[i] {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return s[start : i+1]
			}
		}
	}
	return s[start:]
}

func (p *Planner) buildMessages(state *State, registry *Registry) []api.Message {
	system := p.systemPrompt(registry)
	msgs := make([]api.Message, 0, 2+len(state.Steps))
	msgs = append(msgs, api.Message{Role: "system", Content: system})
	msgs = append(msgs, api.Message{Role: "user", Content: "Goal: " + state.Goal})
	for _, step := range state.Steps {
		msgs = append(msgs, api.Message{Role: step.Role, Content: step.Content})
	}
	return msgs
}

func (p *Planner) systemPrompt(registry *Registry) string {
	toolsDesc := "Available tools (respond with JSON only):\n"
	for _, name := range registry.List() {
		tool := registry.Get(name)
		if tool == nil {
			continue
		}
		var schema []string
		for k, v := range tool.Schema {
			schema = append(schema, k+": "+v)
		}
		toolsDesc += fmt.Sprintf("- %s: %s. Args: %s\n", tool.Name, tool.Description, strings.Join(schema, ", "))
	}
	return "You are an operator investigating a goal. Respond only with a single JSON object, no markdown or explanation. " +
		"Either {\"action\":\"answer\",\"content\":\"...\"} to finish with a summary, or {\"action\":\"tool\",\"name\":\"...\",\"args\":{...},\"reasoning\":\"...\"} to run one tool. " +
		"Do not run destructive commands (e.g. rm -rf, sudo). " +
		toolsDesc
}
