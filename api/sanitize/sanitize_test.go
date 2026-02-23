package sanitize

import (
	"strings"
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		s    string
		want int
	}{
		{"", 0},
		{"a", 1},
		{"abcd", 1},
		{"hello world", 2},
		{string(make([]byte, 40)), 10},
	}
	for _, tt := range tests {
		got := EstimateTokens(tt.s)
		if got != tt.want {
			t.Errorf("EstimateTokens(%q) = %d, want %d", tt.s, got, tt.want)
		}
	}
}

func TestSanitize_LevelNone(t *testing.T) {
	req := Request{Messages: []Message{
		{Role: "system", Content: "You are a bot."},
		{Role: "user", Content: "Hello"},
	}}
	opts := DefaultOptions()
	opts.Level = LevelNone
	out, stats, err := Sanitize(req, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 2 {
		t.Fatalf("expected 2 messages, got %d", len(out.Messages))
	}
	if out.Messages[0].Content != "You are a bot." || out.Messages[1].Content != "Hello" {
		t.Errorf("content changed with level none: %+v", out.Messages)
	}
	if stats.TokensBefore != stats.TokensAfter {
		t.Errorf("tokens should be unchanged: before=%d after=%d", stats.TokensBefore, stats.TokensAfter)
	}
}

func TestSanitize_PreserveLastUser(t *testing.T) {
	critical := "User question: run ls -la"
	req := Request{Messages: []Message{
		{Role: "system", Content: "System with [DEBUG] line"},
		{Role: "user", Content: critical},
	}}
	opts := DefaultOptions()
	opts.Level = LevelLight
	out, _, err := Sanitize(req, opts)
	if err != nil {
		t.Fatal(err)
	}
	// Last user message must be preserved
	if out.Messages[1].Content != critical {
		t.Errorf("last user message must be preserved: got %q", out.Messages[1].Content)
	}
}

func TestSanitize_LightRemovesDebug(t *testing.T) {
	req := Request{Messages: []Message{
		{Role: "system", Content: "System\n[DEBUG] debug line\nReal system content"},
	}}
	opts := DefaultOptions()
	opts.Level = LevelLight
	out, stats, err := Sanitize(req, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	// [DEBUG] line should be removed
	if strings.Contains(out.Messages[0].Content, "[DEBUG]") {
		t.Errorf("light filter should remove [DEBUG] line: got %q", out.Messages[0].Content)
	}
	if !strings.Contains(out.Messages[0].Content, "Real system content") {
		t.Errorf("real content should remain: got %q", out.Messages[0].Content)
	}
	if stats.TokensAfter > stats.TokensBefore {
		t.Errorf("tokens should not increase: before=%d after=%d", stats.TokensBefore, stats.TokensAfter)
	}
}

func TestSanitize_AggressiveRemovesMore(t *testing.T) {
	// Long line with spaces so it's truncated rather than dropped
	longLine := strings.Repeat("word ", 120) // > 500 runes
	req := Request{Messages: []Message{
		{Role: "assistant", Content: "Short.\n\n" + longLine + "\nReal end"},
	}}
	opts := DefaultOptions()
	opts.Level = LevelAggressive
	out, _, err := Sanitize(req, opts)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(out.Messages))
	}
	// Long line should be truncated to ~250 runes + "…"
	if len([]rune(out.Messages[0].Content)) > 280 {
		t.Errorf("aggressive should truncate long lines: len=%d", len([]rune(out.Messages[0].Content)))
	}
}

func TestSanitize_MaxTokensCap(t *testing.T) {
	req := Request{Messages: []Message{
		{Role: "system", Content: "System"},
		{Role: "user", Content: "First user"},
		{Role: "assistant", Content: "Assistant reply"},
		{Role: "user", Content: "Last user message"},
	}}
	opts := DefaultOptions()
	opts.Level = LevelLight
	opts.MaxTokensAfter = 20 // very low to force trimming
	out, stats, err := Sanitize(req, opts)
	if err != nil {
		t.Fatal(err)
	}
	total := 0
	for _, m := range out.Messages {
		total += EstimateTokens(m.Content)
	}
	if total > 25 {
		t.Errorf("total tokens should be capped ~20, got %d", total)
	}
	// Last user message must still be present
	var lastUser string
	for _, m := range out.Messages {
		if m.Role == "user" {
			lastUser = m.Content
		}
	}
	if lastUser != "Last user message" {
		t.Errorf("last user message must be preserved: got %q", lastUser)
	}
	_ = stats
}

func TestSanitize_Performance(t *testing.T) {
	large := string(make([]byte, 50000))
	req := Request{Messages: []Message{
		{Role: "system", Content: large},
		{Role: "user", Content: "Hello"},
	}}
	opts := DefaultOptions()
	opts.Level = LevelAggressive
	opts.MaxDurationMillis = 100
	_, stats, err := Sanitize(req, opts)
	if err != nil {
		t.Fatal(err)
	}
	if stats.DurationMillis > 150 {
		t.Errorf("sanitize should complete within ~100ms, got %d ms", stats.DurationMillis)
	}
}
