package serve

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"gaia/kernel"

	"github.com/modelcontextprotocol/go-sdk/mcp"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

const (
	daemonEnv   = "GAIA_DAEMON"
	defaultPort = "8765"
)

// resolvePort reads serve.port, which was declared and read by nothing.
func resolvePort() string {
	if port := strings.TrimSpace(viper.GetString("serve.port")); port != "" {
		return port
	}
	return defaultPort
}

// endpoint is built from the port actually in use.
func endpoint() string {
	return "http://localhost:" + resolvePort() + "/mcp"
}

// ServePlugin runs an MCP-over-HTTP daemon exposing all plugin tools.
type ServePlugin struct {
	kernel.BasePlugin

	k *kernel.Kernel
}

func NewServePlugin() *ServePlugin { return &ServePlugin{} }

func (p *ServePlugin) ID() string             { return "serve" }
func (p *ServePlugin) DefaultEnabled() bool   { return true }
func (p *ServePlugin) ConfigSchema() []string { return []string{"serve.port"} }

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
		&cobra.Command{
			Use:   "token",
			Short: "Print the bearer token an MCP client must present",
			RunE:  p.runToken,
		},
	)
	return []*cobra.Command{root}, nil
}

// runServe forks the daemon, or is the daemon when GAIA_DAEMON=1.
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
			if err := writeStdoutf(cmd, "gaia serve already running (pid %d)\n", pid); err != nil {
				return err
			}
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

	devNull, err := os.Open(os.DevNull)
	if err != nil {
		_ = logFile.Close()
		return fmt.Errorf("open null device: %w", err)
	}

	child, err := os.StartProcess(exe, []string{exe, "serve"}, &os.ProcAttr{
		Env:   append(os.Environ(), daemonEnv+"=1"),
		Files: []*os.File{devNull, logFile, logFile},
		Sys:   &syscall.SysProcAttr{Setsid: true},
	})
	_ = devNull.Close()
	_ = logFile.Close()
	if err != nil {
		return fmt.Errorf("start daemon: %w", err)
	}

	if err := os.WriteFile(pidFile, []byte(strconv.Itoa(child.Pid)), 0644); err != nil {
		_ = child.Release()
		return fmt.Errorf("write pid: %w", err)
	}
	if err := child.Release(); err != nil {
		return fmt.Errorf("release daemon process: %w", err)
	}

	if err := writeStdoutf(cmd, "gaia serve started (pid %d)\n", child.Pid); err != nil {
		return err
	}
	if err := writeStdoutf(cmd, "MCP endpoint: %s\n", endpoint()); err != nil {
		return err
	}
	if err := writeStdoutf(cmd, "Token: %s\n", tokenPath()); err != nil {
		return err
	}
	if err := writeStdoutf(cmd, "Log: %s\n", logPath()); err != nil {
		return err
	}
	return nil
}

// mcpHandler is split from runDaemon so tests need not bind the real port.
func (p *ServePlugin) mcpHandler() (http.Handler, error) {
	server := mcp.NewServer(&mcp.Implementation{
		Name:    "gaia",
		Version: "1.0.0",
	}, nil)

	// EnabledPlugins: a disabled plugin kept answering over MCP before this.
	for _, plugin := range p.k.EnabledPlugins() {
		for _, tool := range plugin.MCPTools() {
			t := tool // capture loop variable
			server.AddTool(&mcp.Tool{
				Name:        t.Name,
				Description: t.Description,
				InputSchema: t.InputSchema,
			}, func(toolCtx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
				var args map[string]interface{}
				if len(req.Params.Arguments) > 0 {
					// Reported: the handler used to run with none of them.
					if err := json.Unmarshal(req.Params.Arguments, &args); err != nil {
						r := &mcp.CallToolResult{}
						r.SetError(fmt.Errorf("decode arguments for %s: %w", t.Name, err))
						return r, nil
					}
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

	streamable := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, nil)

	token, err := ensureToken(tokenPath())
	if err != nil {
		return nil, err
	}

	mux := http.NewServeMux()
	mux.Handle("/mcp", guard(token, streamable))
	return mux, nil
}

// runDaemon is the long-running server process.
func (p *ServePlugin) runDaemon(ctx context.Context) error {
	handler, err := p.mcpHandler()
	if err != nil {
		return err
	}

	srv := &http.Server{
		Addr:              "localhost:" + resolvePort(),
		Handler:           handler,
		ReadHeaderTimeout: 10 * time.Second,
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
		if err := writeStdoutln(cmd, "gaia serve is not running"); err != nil {
			return err
		}
		return nil
	}
	if !isRunning(pid) {
		_ = os.Remove(pidPath())
		if err := writeStdoutln(cmd, "gaia serve is not running (stale pid removed)"); err != nil {
			return err
		}
		return nil
	}
	proc, _ := os.FindProcess(pid)
	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("kill pid %d: %w", pid, err)
	}
	_ = os.Remove(pidPath())
	if err := writeStdoutf(cmd, "gaia serve stopped (pid %d)\n", pid); err != nil {
		return err
	}
	return nil
}

// runToken prints the token, generating one if the daemon has never run.
func (p *ServePlugin) runToken(cmd *cobra.Command, _ []string) error {
	token, err := ensureToken(tokenPath())
	if err != nil {
		return err
	}
	return writeStdoutln(cmd, token)
}

func (p *ServePlugin) runStatus(cmd *cobra.Command, _ []string) error {
	pid, err := readPID(pidPath())
	if err != nil {
		if err := writeStdoutln(cmd, "gaia serve: stopped"); err != nil {
			return err
		}
		return nil
	}
	if isRunning(pid) {
		if err := writeStdoutf(cmd, "gaia serve: running (pid %d)\n", pid); err != nil {
			return err
		}
		if err := writeStdoutf(cmd, "MCP endpoint: %s\n", endpoint()); err != nil {
			return err
		}
	} else {
		_ = os.Remove(pidPath())
		if err := writeStdoutln(cmd, "gaia serve: stopped (stale pid removed)"); err != nil {
			return err
		}
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

func writeStdoutf(cmd *cobra.Command, format string, args ...interface{}) error {
	_, err := fmt.Fprintf(cmd.OutOrStdout(), format, args...)
	return err
}

func writeStdoutln(cmd *cobra.Command, args ...interface{}) error {
	_, err := fmt.Fprintln(cmd.OutOrStdout(), args...)
	return err
}

func isRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return proc.Signal(syscall.Signal(0)) == nil
}
