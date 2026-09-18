package ask

import (
	"encoding/json"
	"strings"
)

// Smaller models sometimes write the call they mean to make instead of making
// it: a well-formed JSON object, in prose, at the end of the message. The run
// then stops with the answer one step away.

// ToolCallInText recovers the last tool call written as text, if it names a tool
// that exists. Only the last one: everything before it is a model explaining
// what it could do, and running an illustration is not what it asked for.
func ToolCallInText(text string, known []string) (ToolCall, bool) {
	if strings.TrimSpace(text) == "" || len(known) == 0 {
		return ToolCall{}, false
	}
	names := make(map[string]bool, len(known))
	for _, name := range known {
		names[name] = true
	}

	var found ToolCall
	ok := false
	for _, candidate := range jsonObjects(text) {
		call, valid := asToolCall(candidate, names)
		if valid {
			found, ok = call, true
		}
	}
	return found, ok
}

// asToolCall reads the two shapes a model writes: the flat one the tools API
// documents, and the nested one it answers with.
func asToolCall(raw string, known map[string]bool) (ToolCall, bool) {
	var shape struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
		Function  *struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"function"`
	}
	if err := json.Unmarshal([]byte(raw), &shape); err != nil {
		return ToolCall{}, false
	}
	name, args := shape.Name, shape.Arguments
	if shape.Function != nil && strings.TrimSpace(shape.Function.Name) != "" {
		name, args = shape.Function.Name, shape.Function.Arguments
	}
	if !known[strings.TrimSpace(name)] {
		return ToolCall{}, false
	}
	return ToolCall{Name: strings.TrimSpace(name), Arguments: decodeArguments(args)}, true
}

// jsonObjects finds balanced {...} spans, skipping braces inside strings so a
// Go snippet in an argument does not end the object early.
func jsonObjects(text string) []string {
	var out []string
	depth, start := 0, 0
	inString, escaped := false, false

	for i, r := range text {
		switch {
		case escaped:
			escaped = false
		case r == '\\' && inString:
			escaped = true
		case r == '"':
			inString = !inString
		case inString:
		case r == '{':
			if depth == 0 {
				start = i
			}
			depth++
		case r == '}':
			if depth > 0 {
				depth--
				if depth == 0 {
					out = append(out, text[start:i+1])
				}
			}
		}
	}
	return out
}
