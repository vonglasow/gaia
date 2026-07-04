package mempalace

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestFormatStatus_SummarizesNestedPayload(t *testing.T) {
	inner := `{
  "total_drawers": 0,
  "wings": {},
  "rooms": {},
  "palace_path": "/Users/test/.mempalace/palace",
  "protocol": "IMPORTANT very long protocol ...",
  "aaak_dialect": "very long aaak dialect ..."
}`
	raw := `{"content":[{"type":"text","text":` + jsonString(inner) + `}]}`

	out := formatStatus(json.RawMessage(raw))
	if !strings.Contains(out, "palace_path: /Users/test/.mempalace/palace") {
		t.Fatalf("missing palace_path summary: %s", out)
	}
	if !strings.Contains(out, "total_drawers: 0") {
		t.Fatalf("missing total_drawers summary: %s", out)
	}
	if !strings.Contains(out, "wings: 0") || !strings.Contains(out, "rooms: 0") {
		t.Fatalf("missing wings/rooms summary: %s", out)
	}
	if !strings.Contains(out, "protocol: available") || !strings.Contains(out, "aaak_dialect: available") {
		t.Fatalf("missing protocol/aaak markers: %s", out)
	}
	if strings.Contains(out, "IMPORTANT very long protocol") {
		t.Fatalf("long protocol should not be dumped in status output: %s", out)
	}
}

func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}
