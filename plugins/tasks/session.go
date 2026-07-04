package tasks

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ClaudeProjectsDir returns the default Claude Code projects directory.
func ClaudeProjectsDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".claude", "projects")
}

// ReadSessions scans the Claude Code projects directory for sessions newer than since.
// Pass zero time to read all sessions.
func ReadSessions(projectsDir string, since time.Time) ([]SessionSummary, error) {
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}

	var summaries []SessionSummary
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		projPath := filepath.Join(projectsDir, entry.Name())
		files, err := filepath.Glob(filepath.Join(projPath, "*.jsonl"))
		if err != nil {
			continue
		}
		for _, fpath := range files {
			s, err := parseSessionFile(fpath, since)
			if err != nil || s == nil {
				continue
			}
			// Embed the project directory path as a hint for the LLM
			s.ProjectPath = projPath
			summaries = append(summaries, *s)
		}
	}
	return summaries, nil
}

// parseSessionFile reads one JSONL transcript and returns a SessionSummary,
// or nil if the session is older than since or has no user messages.
func parseSessionFile(fpath string, since time.Time) (*SessionSummary, error) {
	f, err := os.Open(fpath)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var (
		firstTS  time.Time
		lastTS   time.Time
		messages []string
	)

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20) // 1MB per line
	for scanner.Scan() {
		var turn map[string]json.RawMessage
		if err := json.Unmarshal(scanner.Bytes(), &turn); err != nil {
			continue
		}

		// Parse timestamp
		var tsStr string
		if raw, ok := turn["timestamp"]; ok {
			_ = json.Unmarshal(raw, &tsStr)
		}
		ts, _ := time.Parse(time.RFC3339, tsStr)

		// Track first/last timestamps
		if !ts.IsZero() {
			if firstTS.IsZero() {
				firstTS = ts
			}
			lastTS = ts
		}

		// Only extract user messages
		var msgType string
		if raw, ok := turn["type"]; ok {
			_ = json.Unmarshal(raw, &msgType)
		}
		if msgType != "user" {
			continue
		}

		text := extractMessageText(turn["message"])
		if text = strings.TrimSpace(text); text != "" {
			messages = append(messages, text)
		}
	}

	if len(messages) == 0 {
		return nil, nil
	}
	if !since.IsZero() && !firstTS.IsZero() && firstTS.Before(since) {
		return nil, nil
	}

	durationMinutes := 0
	if !firstTS.IsZero() && !lastTS.IsZero() && lastTS.After(firstTS) {
		durationMinutes = int(lastTS.Sub(firstTS).Minutes())
	}
	if durationMinutes == 0 {
		durationMinutes = 5 // minimum session duration
	}

	date := "unknown"
	if !firstTS.IsZero() {
		date = firstTS.Format("2006-01-02")
	}

	return &SessionSummary{
		Date:            date,
		DurationMinutes: durationMinutes,
		Messages:        messages,
	}, nil
}

// extractMessageText extracts text from a message JSON object.
// Handles both string content and []{"type":"text","text":"..."} arrays.
func extractMessageText(raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var msg struct {
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(raw, &msg); err != nil || len(msg.Content) == 0 {
		return ""
	}

	// Try string content first
	var s string
	if err := json.Unmarshal(msg.Content, &s); err == nil {
		return s
	}

	// Try array of {type, text} objects
	var parts []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if err := json.Unmarshal(msg.Content, &parts); err == nil {
		var b strings.Builder
		for _, p := range parts {
			if p.Type == "text" {
				b.WriteString(p.Text)
			}
		}
		return b.String()
	}
	return ""
}
