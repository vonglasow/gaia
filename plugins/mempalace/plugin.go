package mempalace

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"gaia/kernel"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"golang.org/x/sys/unix"
)

type MemPalacePlugin struct{}

func NewMemPalacePlugin() *MemPalacePlugin { return &MemPalacePlugin{} }

func (p *MemPalacePlugin) ID() string           { return "mempalace" }
func (p *MemPalacePlugin) DefaultEnabled() bool { return true }
func (p *MemPalacePlugin) DependsOn() []string  { return nil }

func (p *MemPalacePlugin) ConfigSchema() []string {
	return []string{
		"mempalace.mcp.command",
		"mempalace.mcp.args",
		"mempalace.mcp.timeout_seconds",
		"mempalace.debug",
		"mempalace.palace_path",
		"mempalace.inject.enabled",
		"mempalace.inject.max_results",
		"mempalace.inject.min_score",
		"mempalace.diary.enabled",
		"mempalace.context.enabled",
		"mempalace.context.wing",
		"mempalace.context.room",
		"mempalace.context.max_results",
		"mempalace.context.min_score",
	}
}

func (p *MemPalacePlugin) MCPTools() []kernel.MCPTool { return nil }

func (p *MemPalacePlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "mem",
		Short: "MemPalace MCP tools",
	}
	root.PersistentFlags().String("palace-path", "", "Optional MemPalace path")
	_ = viper.BindPFlag("mempalace.palace_path", root.PersistentFlags().Lookup("palace-path"))

	statusCmd := &cobra.Command{
		Use:   "status",
		Short: "Show MemPalace status",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := CallTool(cmd.Context(), "mempalace_status", nil)
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			return shared.PrintBox(cmd.OutOrStdout(), "MemPalace", formatStatus(raw))
		},
	}

	toolsCmd := &cobra.Command{
		Use:   "tools",
		Short: "List discovered MCP tools",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, err := ListTools(cmd.Context())
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			return shared.PrintBox(cmd.OutOrStdout(), "MemPalace Tools", formatRaw(raw))
		},
	}

	callCmd := &cobra.Command{
		Use:   "call [tool] [json-args]",
		Short: "Call any MCP tool dynamically",
		Args:  cobra.RangeArgs(1, 2),
		RunE: func(cmd *cobra.Command, args []string) error {
			tool := strings.TrimSpace(args[0])
			var callArgs map[string]interface{}
			if len(args) == 2 && strings.TrimSpace(args[1]) != "" {
				if err := json.Unmarshal([]byte(args[1]), &callArgs); err != nil {
					return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("invalid json args: %v", err))
				}
			}
			raw, err := CallTool(cmd.Context(), tool, callArgs)
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			return shared.PrintBox(cmd.OutOrStdout(), "MemPalace", formatRaw(raw))
		},
	}

	searchCmd := &cobra.Command{
		Use:   "search [query]",
		Short: "Search MemPalace memories",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.TrimSpace(args[0])
			if query == "" {
				return shared.PrintError(cmd.ErrOrStderr(), "query is required")
			}
			maxResults, minScore := resolveInjectLimits(cmd)
			items, raw, err := searchMemories(cmd.Context(), query, maxResults, minScore)
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			if len(items) == 0 {
				return shared.PrintBox(cmd.OutOrStdout(), "MemPalace", "No results")
			}
			inner := detectSearchInnerWidth(cmd.OutOrStdout())
			return shared.PrintBox(cmd.OutOrStdout(), "MemPalace", formatItemsWrapped(items, raw, inner))
		},
	}

	injectCmd := &cobra.Command{
		Use:   "inject [query]",
		Short: "Render MemPalace results as context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			query := strings.TrimSpace(args[0])
			if query == "" {
				return shared.PrintError(cmd.ErrOrStderr(), "query is required")
			}
			maxResults, minScore := resolveInjectLimits(cmd)
			items, raw, err := searchMemories(cmd.Context(), query, maxResults, minScore)
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			contextBlock := BuildMemoryContext(items, raw)
			if contextBlock == "" {
				return shared.PrintBox(cmd.OutOrStdout(), "Memory", "No results")
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Memory", contextBlock)
		},
	}

	searchCmd.Flags().Int("max-results", 0, "Limit number of results")
	searchCmd.Flags().Float64("min-score", 0, "Minimum score threshold")
	injectCmd.Flags().Int("max-results", 0, "Limit number of results")
	injectCmd.Flags().Float64("min-score", 0, "Minimum score threshold")

	root.AddCommand(statusCmd, toolsCmd, callCmd, searchCmd, injectCmd)
	return []*cobra.Command{root}, nil
}

func resolveInjectLimits(cmd *cobra.Command) (int, float64) {
	maxResults := viper.GetInt("mempalace.inject.max_results")
	if maxResults == 0 {
		maxResults = 5
	}
	minScore := viper.GetFloat64("mempalace.inject.min_score")
	if flag := cmd.Flags().Lookup("max-results"); flag != nil && flag.Changed {
		maxResults, _ = cmd.Flags().GetInt("max-results")
	}
	if flag := cmd.Flags().Lookup("min-score"); flag != nil && flag.Changed {
		minScore, _ = cmd.Flags().GetFloat64("min-score")
	}
	return maxResults, minScore
}

var (
	globalManagerMu sync.Mutex
	globalManager   *Manager
	globalCfgSig    string
)

func managerConfigFromViper() Config {
	timeoutSeconds := viper.GetInt("mempalace.mcp.timeout_seconds")
	if timeoutSeconds <= 0 {
		timeoutSeconds = 30
	}
	command := strings.TrimSpace(viper.GetString("mempalace.mcp.command"))
	if command == "" {
		home, err := os.UserHomeDir()
		if err == nil && strings.TrimSpace(home) != "" {
			command = home + "/.local/pipx/venvs/mempalace/bin/python"
		} else {
			command = "python"
		}
	}
	args := viper.GetStringSlice("mempalace.mcp.args")
	if len(args) == 0 {
		args = []string{"-m", "mempalace.mcp_server"}
	}
	return Config{
		Command:         command,
		Args:            args,
		Timeout:         time.Duration(timeoutSeconds) * time.Second,
		PalacePath:      strings.TrimSpace(viper.GetString("mempalace.palace_path")),
		StatusOnStartup: true,
		Verbose:         viper.GetBool("debug") || viper.GetBool("mempalace.debug"),
	}
}

func configSignature(cfg Config) string {
	parts := []string{
		cfg.Command,
		strings.Join(cfg.Args, "\x00"),
		cfg.Timeout.String(),
		cfg.PalacePath,
		fmt.Sprintf("%t", cfg.Verbose),
	}
	return strings.Join(parts, "|")
}

func getManager() *Manager {
	cfg := managerConfigFromViper().normalized()
	sig := configSignature(cfg)

	globalManagerMu.Lock()
	defer globalManagerMu.Unlock()
	if globalManager != nil && globalCfgSig == sig {
		return globalManager
	}
	if globalManager != nil {
		_ = globalManager.Close()
	}
	globalManager = NewManager(cfg)
	globalCfgSig = sig
	return globalManager
}

func CallTool(ctx context.Context, name string, args map[string]interface{}) (json.RawMessage, error) {
	return getManager().CallTool(ctx, name, args)
}

func ListTools(ctx context.Context) (json.RawMessage, error) {
	return getManager().ListTools(ctx)
}

func formatRaw(raw json.RawMessage) string {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return "(empty)"
	}
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, raw, "", "  "); err != nil {
		return string(raw)
	}
	return pretty.String()
}

func detectSearchInnerWidth(w io.Writer) int {
	if f, ok := w.(*os.File); ok {
		ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
		if err == nil && ws != nil && ws.Col > 0 {
			// RenderBox adds 2 border columns and PrintBox adds one trailing newline.
			inner := int(ws.Col) - 2
			if inner >= 20 {
				return inner
			}
		}
	}
	if col := strings.TrimSpace(os.Getenv("COLUMNS")); col != "" {
		if n, err := strconv.Atoi(col); err == nil && n > 0 {
			inner := n - 2
			if inner >= 20 {
				return inner
			}
		}
	}
	// Safe fallback when size cannot be detected.
	return 78
}

func formatStatus(raw json.RawMessage) string {
	statusObj, ok := parseStatusPayload(raw)
	if !ok {
		return formatRaw(raw)
	}

	lines := []string{}
	if v, ok := statusObj["palace_path"].(string); ok && strings.TrimSpace(v) != "" {
		lines = append(lines, "palace_path: "+strings.TrimSpace(v))
	}
	if n, ok := asInt(statusObj["total_drawers"]); ok {
		lines = append(lines, fmt.Sprintf("total_drawers: %d", n))
	}
	if n, ok := mapLen(statusObj["wings"]); ok {
		lines = append(lines, fmt.Sprintf("wings: %d", n))
	}
	if n, ok := mapLen(statusObj["rooms"]); ok {
		lines = append(lines, fmt.Sprintf("rooms: %d", n))
	}
	if _, ok := statusObj["protocol"].(string); ok {
		lines = append(lines, "protocol: available")
	}
	if _, ok := statusObj["aaak_dialect"].(string); ok {
		lines = append(lines, "aaak_dialect: available")
	}
	if len(lines) == 0 {
		return formatRaw(raw)
	}
	return strings.Join(lines, "\n")
}

func parseStatusPayload(raw json.RawMessage) (map[string]interface{}, bool) {
	raw = json.RawMessage(strings.TrimSpace(string(raw)))
	if len(raw) == 0 {
		return nil, false
	}

	var direct map[string]interface{}
	if err := json.Unmarshal(raw, &direct); err == nil {
		if _, has := direct["total_drawers"]; has {
			return direct, true
		}
	}

	var envelope struct {
		Content []struct {
			Type string `json:"type"`
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return nil, false
	}
	for _, item := range envelope.Content {
		if strings.TrimSpace(item.Text) == "" {
			continue
		}
		var inner map[string]interface{}
		if err := json.Unmarshal([]byte(item.Text), &inner); err == nil {
			if _, has := inner["total_drawers"]; has {
				return inner, true
			}
		}
	}
	return nil, false
}

func asInt(v interface{}) (int, bool) {
	switch n := v.(type) {
	case int:
		return n, true
	case int64:
		return int(n), true
	case float64:
		return int(n), true
	default:
		return 0, false
	}
}

func mapLen(v interface{}) (int, bool) {
	m, ok := v.(map[string]interface{})
	if !ok {
		return 0, false
	}
	return len(m), true
}
