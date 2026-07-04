package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"gaia/kernel"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
)

const (
	daemonEnv   = "GAIA_DAEMON"
	defaultPort = "8765"
)

// ServePlugin runs an MCP-over-HTTP daemon exposing all plugin tools.
type ServePlugin struct {
	k *kernel.Kernel
}

func NewServePlugin() *ServePlugin { return &ServePlugin{} }

func (p *ServePlugin) ID() string             { return "serve" }
func (p *ServePlugin) DefaultEnabled() bool   { return true }
func (p *ServePlugin) DependsOn() []string    { return nil }
func (p *ServePlugin) ConfigSchema() []string { return []string{"serve.port"} }
func (p *ServePlugin) MCPTools() []kernel.MCPTool { return nil }

func (p *ServePlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	p.k = k

	root := &cobra.Command{
		Use:   "serve",
		Short: "Start gaia as a Streamable HTTP MCP server daemon",
		RunE:  p.runServe,
	}
	root.AddCommand(
		&cobra.Command{
			Use:   "stop",
			Short: "Stop the gaia MCP server daemon",
			RunE:  p.runStop,
		},
		&cobra.Command{
			Use:   "status",
			Short: "Show gaia MCP server daemon status",
			RunE:  p.runStatus,
		},
	)
	return []*cobra.Command{root}, nil
}

// runServe is the entry point. In the parent process it forks the daemon;
// in the child (GAIA_DAEMON=1) it runs the MCP HTTP server.
func (p *ServePlugin) runServe(cmd *cobra.Command, _ []string) error {
	if os.Getenv(daemonEnv) == "1" {
		return p.runDaemon(cmd.Context())
	}
	return p.forkDaemon(cmd)
}

// forkDaemon re-executes the current binary with GAIA_DAEMON=1, detached.
func (p *ServePlugin) forkDaemon(cmd *cobra.Command) error {
	pidFile := pidPath()
	if pid, err := readPID(pidFile); err == nil {
		if isRunning(pid) {
			fmt.Fprintf(cmd.OutOrStdout(), "gaia serve already running (pid %d)\n", pid)
			return nil
		}
	}

	if err := os.MkdirAll(configDir(), 0755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	logFile, err := os.OpenFile(logPath(), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return fmt.Errorf("open log file: %w", err)
	}

	exe, err := os.Executable()
	if err != nil {
		_ = logFile.Close()
		return fmt.Errorf("executable path: %w", err)
	}

	child := exec.Command(exe, "serve")
	child.Env = append(os.Environ(), daemonEnv+"=1")
	child.Stdout = logFile
	child.Stderr = logFile
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}

	if err := child.Start(); err != nil {
		_ = logFile.Close()
		return fmt.Errorf("start daemon: %w", err)
	}
	_ = logFile.Close()

	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(child.Process.Pid)), 0644); err != nil {
		return fmt.Errorf("write pid: %w", err)
	}

	fmt.Fprintf(cmd.OutOrStdout(), "gaia serve started (pid %d)\n", child.Process.Pid)
	fmt.Fprintf(cmd.OutOrStdout(), "MCP endpoint: http://localhost:%s/mcp\n", defaultPort)
	fmt.Fprintf(cmd.OutOrStdout(), "Log: %s\n", logPath())
	return nil
}

// runDaemon is the long-running server process.
func (p *ServePlugin) runDaemon(ctx context.Context) error {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gaia",
		Version: "1.0.0",
	}, nil)

	for _, plugin := range p.k.Plugins() {
		for _, tool := range plugin.MCPTools() {
			t := tool // capture loop variable
			server.AddTool(&mcp.Tool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			}, func(toolCtx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var args map[string]interface{}
				if len(req.Params.Arguments) > 0 {
					_ = json.Unmarshal(req.Params.Arguments, &args)
				}
				text, err := t.Handler(toolCtx, args)
				if err != nil {
					r := &mcp.CallToolResult{}
					r.SetError(err)
					return r, nil
				}
				return &mcp.CallToolResult{
					Content: []mcp.Content{&mcp.TextContent{Text: text}},
				}, nil
			})
		}
	}

	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)

	mux := http.NewServeMux()
	mux.Handle("/mcp", handler)

	srv := &http.Server{
		Addr:    "localhost:" + defaultPort,
		Handler: mux,
	}

	go func() {
		<-ctx.Done()
		_ = srv.Shutdown(context.Background())
	}()

	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func (p *ServePlugin) runStop(cmd *cobra.Command, _ []string) error {
	pid, err := readPID(pidPath())
	if err != nil {
		fmt.Fprintln(cmd.OutOrStdout(), "gaia serve is not running")
		return nil
	}
	if !isRunning(pid) {
		_ = os.Remove(pidPath())
		fmt.Fprintln(cmd.OutOrStdout(), "gaia serve is not running (stale pid removed)")
		return nil
	}
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("kill pid %d: %w", pid, err)
	}
	_ = os.Remove(pidPath())
	fmt.Fprintf(cmd.OutOrStdout(), "gaia serve stopped (pid %d)\n", pid)
	return nil
}

func (p *ServePlugin) runStatus(cmd *cobra.Command, _ []string) error {
	pid, err := readPID(pidPath())
	if err != nil {
		fmt.Fprintln(cmd.OutOrStdout(), "gaia serve: stopped")
		return nil
	}
	if isRunning(pid) {
		fmt.Fprintf(cmd.OutOrStdout(), "gaia serve: running (pid %d)\n", pid)
		fmt.Fprintf(cmd.OutOrStdout(), "MCP endpoint: http://localhost:%s/mcp\n", defaultPort)
	} else {
		_ = os.Remove(pidPath())
		fmt.Fprintln(cmd.OutOrStdout(), "gaia serve: stopped (stale pid removed)")
	}
	return nil
}

// --- path helpers ---

func configDir() string {
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "gaia")
}

func pidPath() string { return filepath.Join(configDir(), "serve.pid") }
func logPath() string { return filepath.Join(configDir(), "serve.log") }

func readPID(path string) (int, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(strings.TrimSpace(string(b)))
}

func isRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
