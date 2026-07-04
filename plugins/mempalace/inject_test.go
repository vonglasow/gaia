package mempalace

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
)

func TestParseMemoryItems(t *testing.T) {
	raw := []byte(`{"results":[{"text":"foo","score":0.9,"wing":"w","room":"r","id":"1"}]}`)
	items := parseMemoryItems(raw)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Text != "foo" || items[0].Wing != "w" || items[0].Room != "r" || items[0].ID != "1" {
		t.Fatalf("unexpected item: %+v", items[0])
	}
}

func TestParseMemoryItems_FromToolContentEnvelope(t *testing.T) {
	raw := []byte(`{"content":[{"type":"text","text":"{\"results\":[{\"text\":\"hello from drawer\",\"score\":0.91}]}"}]}`)
	items := parseMemoryItems(raw)
	if len(items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(items))
	}
	if items[0].Text != "hello from drawer" {
		t.Fatalf("unexpected text: %+v", items[0])
	}
}

func TestBuildMemoryContext(t *testing.T) {
	items := []MemoryItem{{Text: "hello", Score: 0.5}}
	context := BuildMemoryContext(items, nil)
	if !strings.Contains(context, "Memory Context") {
		t.Fatalf("expected memory context header")
	}
	if !strings.Contains(context, "hello") {
		t.Fatalf("expected item text")
	}
}

func TestFormatItemsFallback(t *testing.T) {
	raw, _ := json.Marshal(map[string]string{"foo": "bar"})
	out := formatItems(nil, raw)
	if !strings.Contains(out, "foo") {
		t.Fatalf("expected raw json")
	}
}

func TestFormatItemsWrapped_WidthConstraint(t *testing.T) {
	items := []MemoryItem{
		{
			Text:  "This is a very long line that should wrap within the terminal width and never overflow outside the rendered box.",
			Score: 0.9,
			Wing:  "gaia",
			Room:  "ask",
		},
	}
	out := formatItemsWrapped(items, nil, 78)
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 78 {
			t.Fatalf("line exceeds width: %d > 78, line=%q", lipgloss.Width(line), line)
		}
	}
}

func TestFormatItemsWrapped_NoTruncation(t *testing.T) {
	long := "alpha beta gamma delta epsilon zeta eta theta iota kappa lambda mu nu xi omicron pi rho sigma tau upsilon phi chi psi omega"
	items := []MemoryItem{{Text: long, Wing: "gaia", Room: "ask"}}
	out := formatItemsWrapped(items, nil, 40)
	for _, token := range strings.Fields(long) {
		if !strings.Contains(out, token) {
			t.Fatalf("expected token %q in output", token)
		}
	}
	if strings.Contains(out, "...") {
		t.Fatalf("unexpected truncation marker in output: %q", out)
	}
}

func TestFormatItemsWrapped_CodeBlockIntegrity(t *testing.T) {
	text := "Here is code:\n```python\nprint('hello world from a very long line that should wrap inside code block safely')\n```"
	items := []MemoryItem{{Text: text, Wing: "gaia", Room: "ask"}}
	out := formatItemsWrapped(items, nil, 50)
	if strings.Count(out, "```") < 2 {
		t.Fatalf("expected code fences preserved, got: %q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 50 {
			t.Fatalf("line exceeds width: %d > 50, line=%q", lipgloss.Width(line), line)
		}
	}
}

func TestFormatItemsWrapped_MultiResultIndependentWrap(t *testing.T) {
	items := []MemoryItem{
		{Text: "first result with long content that must wrap independently", Score: 0.9, Wing: "gaia", Room: "ask"},
		{Text: "second result with another long content that must also wrap independently", Score: 0.8, Wing: "gaia", Room: "chat"},
	}
	out := formatItemsWrapped(items, nil, 60)
	if strings.Count(out, "- ") < 2 {
		t.Fatalf("expected two rendered results, got: %q", out)
	}
	if !strings.Contains(out, "first result") || !strings.Contains(out, "second result") {
		t.Fatalf("expected both result contents, got: %q", out)
	}
	for _, line := range strings.Split(out, "\n") {
		if lipgloss.Width(line) > 60 {
			t.Fatalf("line exceeds width: %d > 60, line=%q", lipgloss.Width(line), line)
		}
	}
}

func TestPersistAskResponse_CallsAddDrawer(t *testing.T) {
	prevCall := callToolFn
	callToolFn = func(_ context.Context, name string, args map[string]interface{}) (json.RawMessage, error) {
		if name != "mempalace_add_drawer" {
			t.Fatalf("unexpected tool name: %s", name)
		}
		if args["wing"] != "gaia" || args["room"] != "ask" {
			t.Fatalf("unexpected wing/room: %#v", args)
		}
		if args["content"] != "answer text" {
			t.Fatalf("unexpected content: %#v", args["content"])
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
	t.Cleanup(func() {
		callToolFn = prevCall
	})

	if err := PersistAskResponse(context.Background(), "hello", "answer text"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPersistChatTurn_UsesChatRoom(t *testing.T) {
	prevCall := callToolFn
	callToolFn = func(_ context.Context, name string, args map[string]interface{}) (json.RawMessage, error) {
		if name != "mempalace_add_drawer" {
			t.Fatalf("unexpected tool name: %s", name)
		}
		if args["room"] != "chat" || args["wing"] != "gaia" {
			t.Fatalf("unexpected room/wing: %#v", args)
		}
		if !strings.Contains(args["content"].(string), "assistant: hello back") {
			t.Fatalf("unexpected content: %#v", args["content"])
		}
		return json.RawMessage(`{"ok":true}`), nil
	}
	t.Cleanup(func() {
		callToolFn = prevCall
	})

	if err := PersistChatTurn(context.Background(), "s1", 2, "hello", "hello back"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}
