package mempalace

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestManager_StatusAndSearch(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_MCP") == "1" {
		helperMCPServer()
		return
	}

	t.Setenv("GO_WANT_HELPER_MCP", "1")
	t.Setenv("GO_WANT_HELPER_MCP_MODE", "ok")
	startsFile := filepath.Join(t.TempDir(), "starts.txt")
	t.Setenv("GO_WANT_HELPER_MCP_STARTS_FILE", startsFile)
	cfg := Config{
		Command:         os.Args[0],
		Args:            []string{"-test.run=TestManager_StatusAndSearch"},
		Timeout:         2 * time.Second,
		StatusOnStartup: true,
	}
	m := NewManager(cfg)
	defer func() {
		_ = m.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	status, err := m.CallTool(ctx, "mempalace_status", nil)
	if err != nil {
		t.Fatalf("status call failed: %v", err)
	}
	if !strings.Contains(string(status), "ok") {
		t.Fatalf("unexpected status payload: %s", string(status))
	}

	search, err := m.CallTool(ctx, "mempalace_search", map[string]interface{}{"query": "security"})
	if err != nil {
		t.Fatalf("search call failed: %v", err)
	}
	if !strings.Contains(string(search), "security-note") {
		t.Fatalf("unexpected search payload: %s", string(search))
	}

	// Manager should keep one MCP process alive for multiple calls.
	data, err := os.ReadFile(startsFile)
	if err != nil {
		t.Fatalf("read starts file: %v", err)
	}
	count, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		t.Fatalf("parse starts file: %v", err)
	}
	if count != 1 {
		t.Fatalf("expected exactly one MCP process start, got %d", count)
	}
}

func TestManager_InvalidJSONFails(t *testing.T) {
	if os.Getenv("GO_WANT_HELPER_MCP") == "1" {
		helperMCPServer()
		return
	}

	t.Setenv("GO_WANT_HELPER_MCP", "1")
	t.Setenv("GO_WANT_HELPER_MCP_MODE", "invalid-json")
	cfg := Config{
		Command:         os.Args[0],
		Args:            []string{"-test.run=TestManager_InvalidJSONFails"},
		Timeout:         2 * time.Second,
		StatusOnStartup: false,
	}
	m := NewManager(cfg)
	defer func() {
		_ = m.Close()
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := m.CallTool(ctx, "mempalace_status", nil)
	if err == nil {
		t.Fatalf("expected invalid json error")
	}
	lower := strings.ToLower(err.Error())
	if !strings.Contains(lower, "invalid json") && !strings.Contains(lower, "eof") {
		t.Fatalf("expected invalid json/eof error, got: %v", err)
	}
}

func TestResolveCommand_DefaultsToMCPServer(t *testing.T) {
	spec := resolveCommand(Config{})
	if strings.Join(spec.args, " ") != "-m mempalace.mcp_server" {
		t.Fatalf("unexpected default args: %v", spec.args)
	}
	home, err := os.UserHomeDir()
	if err == nil && strings.TrimSpace(home) != "" {
		want := home + "/.local/pipx/venvs/mempalace/bin/python"
		if spec.command != want {
			t.Fatalf("unexpected default command: %s", spec.command)
		}
	}
}

func helperMCPServer() {
	incrementStartsCounter()
	mode := strings.TrimSpace(os.Getenv("GO_WANT_HELPER_MCP_MODE"))
	if mode == "invalid-json" {
		_, _ = os.Stdout.WriteString("{invalid json\n")
		os.Exit(0)
	}

	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		var req map[string]interface{}
		if err := json.Unmarshal(line, &req); err != nil {
			continue
		}
		method, _ := req["method"].(string)
		idVal, hasID := req["id"]
		if !hasID {
			// ignore notifications
			continue
		}
		idNum, _ := idVal.(float64)
		id := int64(idNum)

		resp := map[string]interface{}{
			"jsonrpc": "2.0",
			"id":      id,
		}
		switch method {
		case "initialize":
			resp["result"] = map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"serverInfo":      map[string]interface{}{"name": "fake-mempalace", "version": "1"},
			}
		case "tools/list":
			resp["result"] = map[string]interface{}{
				"tools": []map[string]interface{}{
					{"name": "mempalace_status"},
					{"name": "mempalace_search"},
				},
			}
		case "tools/call":
			params, _ := req["params"].(map[string]interface{})
			name, _ := params["name"].(string)
			switch name {
			case "mempalace_status":
				resp["result"] = map[string]interface{}{"status": "ok"}
			case "mempalace_search":
				resp["result"] = map[string]interface{}{
					"results": []map[string]interface{}{
						{"text": "security-note", "score": 0.95},
					},
				}
			default:
				resp["error"] = map[string]interface{}{"code": -32601, "message": "tool not found"}
			}
		default:
			resp["error"] = map[string]interface{}{"code": -32601, "message": "method not found"}
		}
		enc := json.NewEncoder(os.Stdout)
		_ = enc.Encode(resp)
	}
	os.Exit(0)
}

func incrementStartsCounter() {
	p := strings.TrimSpace(os.Getenv("GO_WANT_HELPER_MCP_STARTS_FILE"))
	if p == "" {
		return
	}
	current := 0
	if raw, err := os.ReadFile(p); err == nil {
		if v, convErr := strconv.Atoi(strings.TrimSpace(string(raw))); convErr == nil {
			current = v
		}
	}
	_ = os.WriteFile(p, []byte(strconv.Itoa(current+1)), 0o600)
}
