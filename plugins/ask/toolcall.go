package ask

import (
	"encoding/json"
	"fmt"
	"strings"
)

// ToolSpec describes one tool to a model: name, purpose, and a JSON Schema.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// ToolCall is a model asking for a tool to be run.
type ToolCall struct {
	Name      string
	Arguments map[string]any
}

// ArgString renders any JSON type, but refuses a missing argument rather than guessing.
func (c ToolCall) ArgString(name string) (string, error) {
	raw, ok := c.Arguments[name]
	if !ok || raw == nil {
		return "", fmt.Errorf("%s: missing argument %q", c.Name, name)
	}
	switch v := raw.(type) {
	case string:
		return v, nil
	case float64:
		// JSON has one number type, so an integer arrives as a float.
		if v == float64(int64(v)) {
			return fmt.Sprintf("%d", int64(v)), nil
		}
		return fmt.Sprintf("%v", v), nil
	case bool:
		return fmt.Sprintf("%t", v), nil
	default:
		encoded, err := json.Marshal(v)
		if err != nil {
			return "", fmt.Errorf("%s: argument %q has a shape that cannot be read: %w", c.Name, name, err)
		}
		return string(encoded), nil
	}
}

// ArgStringOr falls back where a default beats an error: a path meaning "here".
func (c ToolCall) ArgStringOr(name, fallback string) string {
	value, err := c.ArgString(name)
	if err != nil || strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}

// --- the wire format -------------------------------------------------------

// ollamaTool is a tool as Ollama's /api/chat expects it.
type ollamaTool struct {
	Type     string             `json:"type"`
	Function ollamaToolFunction `json:"function"`
}

type ollamaToolFunction struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Parameters  map[string]any `json:"parameters"`
}

// ollamaToolCall is a tool call as Ollama returns it.
type ollamaToolCall struct {
	Function struct {
		Name string `json:"name"`
		// Raw, because some builds send an object and others a JSON string.
		Arguments json.RawMessage `json:"arguments"`
	} `json:"function"`
}

// toOllamaTools renders the specs for the wire.
func toOllamaTools(specs []ToolSpec) []ollamaTool {
	if len(specs) == 0 {
		return nil
	}
	out := make([]ollamaTool, 0, len(specs))
	for _, spec := range specs {
		params := spec.Parameters
		if params == nil {
			// A tool with no arguments still needs a schema to be well-formed.
			params = map[string]any{"type": "object", "properties": map[string]any{}}
		}
		out = append(out, ollamaTool{
			Type: "function",
			Function: ollamaToolFunction{
				Name:        spec.Name,
				Description: spec.Description,
				Parameters:  params,
			},
		})
	}
	return out
}

// parseToolCalls reads what came back, tolerating both argument shapes.
func parseToolCalls(raw []ollamaToolCall) []ToolCall {
	if len(raw) == 0 {
		return nil
	}
	out := make([]ToolCall, 0, len(raw))
	for _, call := range raw {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		out = append(out, ToolCall{
			Name:      call.Function.Name,
			Arguments: decodeArguments(call.Function.Arguments),
		})
	}
	return out
}

// decodeArguments: unreadable becomes empty, so the tool names what it is missing.
func decodeArguments(raw json.RawMessage) map[string]any {
	if len(raw) == 0 {
		return map[string]any{}
	}
	var asObject map[string]any
	if err := json.Unmarshal(raw, &asObject); err == nil {
		return asObject
	}
	var asString string
	if err := json.Unmarshal(raw, &asString); err == nil {
		if err := json.Unmarshal([]byte(asString), &asObject); err == nil {
			return asObject
		}
	}
	return map[string]any{}
}

// buildToolMessages keeps calls and tool names structured, so results stay paired.
func buildToolMessages(req AskRequest) []map[string]any {
	out := []map[string]any{}
	if strings.TrimSpace(req.SystemPrompt) != "" {
		out = append(out, map[string]any{"role": "system", "content": req.SystemPrompt})
	}
	for _, msg := range req.Messages {
		if strings.TrimSpace(msg.Role) == "" {
			continue
		}
		turn := map[string]any{"role": msg.Role, "content": msg.Content}
		if len(msg.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(msg.ToolCalls))
			for _, call := range msg.ToolCalls {
				args := call.Arguments
				if args == nil {
					args = map[string]any{}
				}
				calls = append(calls, map[string]any{
					"function": map[string]any{"name": call.Name, "arguments": args},
				})
			}
			turn["tool_calls"] = calls
		}
		if strings.TrimSpace(msg.ToolName) != "" {
			turn["tool_name"] = msg.ToolName
		}
		// A turn with neither says nothing and confuses the alternation.
		if strings.TrimSpace(msg.Content) == "" && len(msg.ToolCalls) == 0 {
			continue
		}
		out = append(out, turn)
	}
	if strings.TrimSpace(req.Message) != "" {
		out = append(out, map[string]any{"role": "user", "content": req.Message})
	}
	return out
}
