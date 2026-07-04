package mempalace

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"os/exec"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	errClientClosed = errors.New("mcp client closed")
)

// Config controls process startup, retries, and request behavior.
type Config struct {
	Command         string
	Args            []string
	Timeout         time.Duration
	PalacePath      string
	StatusOnStartup bool
	Verbose         bool
}

type commandSpec struct {
	command string
	args    []string
}

// Manager owns a single MCP process and supports concurrent RPC calls.
type Manager struct {
	cfg Config

	mu     sync.Mutex
	client *rpcClient
}

type rpcClient struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout io.ReadCloser
	stderr io.ReadCloser

	writeMu sync.Mutex
	reqID   atomic.Int64

	mu      sync.Mutex
	pending map[int64]chan rpcResult
	closed  bool
	err     error
	done    chan struct{}
}

type rpcRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      int64       `json:"id,omitempty"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type rpcResult struct {
	result json.RawMessage
	err    error
}

type callParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

var mcpLogger = log.New(os.Stderr, "[mempalace] ", log.LstdFlags)
var mcpLogEnabled atomic.Bool

func logEvent(event string, fields map[string]interface{}) {
	if !mcpLogEnabled.Load() {
		return
	}
	if fields == nil {
		fields = map[string]interface{}{}
	}
	fields["event"] = event
	b, err := json.Marshal(fields)
	if err != nil {
		mcpLogger.Printf("event=%s marshal_error=%v", event, err)
		return
	}
	mcpLogger.Println(string(b))
}

func defaultConfig() Config {
	home, err := os.UserHomeDir()
	defaultCmd := "python"
	if err == nil && strings.TrimSpace(home) != "" {
		defaultCmd = home + "/.local/pipx/venvs/mempalace/bin/python"
	}
	return Config{
		Command:         defaultCmd,
		Args:            []string{"-m", "mempalace.mcp_server"},
		Timeout:         30 * time.Second,
		StatusOnStartup: true,
	}
}

func (c Config) normalized() Config {
	n := c
	d := defaultConfig()
	if strings.TrimSpace(n.Command) == "" {
		n.Command = d.Command
	}
	if len(n.Args) == 0 {
		n.Args = append([]string{}, d.Args...)
	}
	if n.Timeout <= 0 {
		n.Timeout = d.Timeout
	}
	return n
}

func NewManager(cfg Config) *Manager {
	n := cfg.normalized()
	mcpLogEnabled.Store(n.Verbose)
	return &Manager{cfg: n}
}

func (m *Manager) Close() error {
	m.mu.Lock()
	c := m.client
	m.client = nil
	m.mu.Unlock()
	if c == nil {
		return nil
	}
	return c.closeGracefully()
}

func (m *Manager) ListTools(ctx context.Context) (json.RawMessage, error) {
	return m.call(ctx, "tools/list", map[string]interface{}{})
}

func (m *Manager) CallTool(ctx context.Context, name string, args map[string]interface{}) (json.RawMessage, error) {
	if strings.TrimSpace(name) == "" {
		return nil, fmt.Errorf("tool name is required")
	}
	return m.call(ctx, "tools/call", callParams{Name: name, Arguments: args})
}

func (m *Manager) call(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	c, err := m.ensureClient(ctx)
	if err != nil {
		return nil, err
	}
	timeout := m.cfg.Timeout
	callCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	payload := ""
	if b, err := json.Marshal(params); err == nil {
		payload = string(b)
	}
	tool := ""
	if cp, ok := params.(callParams); ok {
		tool = cp.Name
	}
	start := time.Now()
	logEvent("request_send", map[string]interface{}{
		"method":  method,
		"tool":    tool,
		"payload": payload,
	})
	raw, callErr := c.request(callCtx, method, params)
	latencyMs := time.Since(start).Milliseconds()
	if callErr != nil {
		logEvent("response_error", map[string]interface{}{
			"method":     method,
			"tool":       tool,
			"latency_ms": latencyMs,
			"error":      callErr.Error(),
		})
		return nil, callErr
	}
	logEvent("response_ok", map[string]interface{}{
		"method":     method,
		"tool":       tool,
		"latency_ms": latencyMs,
	})
	return raw, nil
}

func (m *Manager) ensureClient(ctx context.Context) (*rpcClient, error) {
	m.mu.Lock()
	if m.client != nil && !m.client.isClosed() {
		c := m.client
		m.mu.Unlock()
		return c, nil
	}
	m.mu.Unlock()

	spec := resolveCommand(m.cfg)
	c, err := startRPCClient(spec, m.cfg.PalacePath)
	if err != nil {
		logEvent("process_start_failed", map[string]interface{}{"command": spec.command, "args": strings.Join(spec.args, " "), "error": err.Error()})
		return nil, err
	}
	if err := c.initialize(ctx, m.cfg.Timeout); err != nil {
		_ = c.closeGracefully()
		logEvent("initialize_failed", map[string]interface{}{"command": spec.command, "error": err.Error()})
		return nil, err
	}
	if m.cfg.StatusOnStartup {
		if _, err := c.callTool(ctx, "mempalace_status", nil, m.cfg.Timeout); err != nil {
			_ = c.closeGracefully()
			logEvent("status_failed", map[string]interface{}{"error": err.Error()})
			return nil, err
		}
	}
	m.mu.Lock()
	m.client = c
	m.mu.Unlock()
	logEvent("process_started", map[string]interface{}{"command": spec.command, "args": strings.Join(spec.args, " ")})
	return c, nil
}

func resolveCommand(cfg Config) commandSpec {
	command := strings.TrimSpace(cfg.Command)
	if command == "" {
		command = defaultConfig().Command
	}
	args := append([]string{}, cfg.Args...)
	if len(args) == 0 {
		args = []string{"-m", "mempalace.mcp_server"}
	}
	return commandSpec{command: command, args: args}
}

func startRPCClient(spec commandSpec, palacePath string) (*rpcClient, error) {
	cmd := exec.Command(spec.command, spec.args...) // nosemgrep
	if palacePath == "" {
		palacePath = strings.TrimSpace(os.Getenv("MEMPALACE_PALACE_PATH"))
	}
	if palacePath != "" {
		env := append([]string{}, os.Environ()...)
		env = append(env, "MEMPALACE_PALACE_PATH="+palacePath)
		cmd.Env = env
	}
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	c := &rpcClient{
		cmd:     cmd,
		stdin:   stdin,
		stdout:  stdout,
		stderr:  stderr,
		pending: map[int64]chan rpcResult{},
		done:    make(chan struct{}),
	}
	go c.readLoop()
	go c.stderrLoop()
	go c.waitLoop()
	return c, nil
}

func (c *rpcClient) initialize(ctx context.Context, timeout time.Duration) error {
	params := map[string]interface{}{
		"protocolVersion": "2024-11-05",
		"clientInfo":      map[string]string{"name": "gaia", "version": "dev"},
		"capabilities":    map[string]interface{}{},
	}
	callCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	if _, err := c.request(callCtx, "initialize", params); err != nil {
		return err
	}
	return c.notify("notifications/initialized", map[string]interface{}{})
}

func (c *rpcClient) callTool(ctx context.Context, name string, args map[string]interface{}, timeout time.Duration) (json.RawMessage, error) {
	callCtx := ctx
	if timeout > 0 {
		var cancel context.CancelFunc
		callCtx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return c.request(callCtx, "tools/call", callParams{Name: name, Arguments: args})
}

func (c *rpcClient) request(ctx context.Context, method string, params interface{}) (json.RawMessage, error) {
	if c.isClosed() {
		return nil, errClientClosed
	}
	id := c.reqID.Add(1)
	respCh := make(chan rpcResult, 1)

	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, errClientClosed
	}
	c.pending[id] = respCh
	c.mu.Unlock()

	req := rpcRequest{JSONRPC: "2.0", ID: id, Method: method, Params: params}
	if err := c.write(req); err != nil {
		c.removePending(id)
		return nil, err
	}

	select {
	case <-ctx.Done():
		c.removePending(id)
		return nil, ctx.Err()
	case res := <-respCh:
		return res.result, res.err
	case <-c.done:
		return nil, c.closeError()
	}
}

func (c *rpcClient) notify(method string, params interface{}) error {
	req := rpcRequest{JSONRPC: "2.0", Method: method, Params: params}
	return c.write(req)
}

func (c *rpcClient) write(req rpcRequest) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.isClosed() {
		return errClientClosed
	}
	enc := json.NewEncoder(c.stdin)
	if err := enc.Encode(req); err != nil {
		c.fail(err)
		return err
	}
	return nil
}

func (c *rpcClient) readLoop() {
	dec := json.NewDecoder(c.stdout)
	for {
		var msg map[string]json.RawMessage
		if err := dec.Decode(&msg); err != nil {
			if errors.Is(err, io.EOF) {
				c.fail(io.EOF)
				return
			}
			c.fail(fmt.Errorf("invalid json: %w", err))
			return
		}
		idRaw, hasID := msg["id"]
		if !hasID {
			// notification/event
			continue
		}
		var id int64
		if err := json.Unmarshal(idRaw, &id); err != nil {
			continue
		}
		var rpcErr *rpcError
		if rawErr, ok := msg["error"]; ok && len(rawErr) > 0 {
			_ = json.Unmarshal(rawErr, &rpcErr)
		}
		var result json.RawMessage
		if rawRes, ok := msg["result"]; ok {
			result = rawRes
		}
		c.mu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if ch == nil {
			continue
		}
		if rpcErr != nil {
			ch <- rpcResult{err: fmt.Errorf("mcp error %d: %s", rpcErr.Code, rpcErr.Message)}
			continue
		}
		ch <- rpcResult{result: result}
	}
}

func (c *rpcClient) stderrLoop() {
	data, err := io.ReadAll(c.stderr)
	if err != nil || len(data) == 0 {
		return
	}
	logEvent("process_stderr", map[string]interface{}{"stderr": strings.TrimSpace(string(data))})
}

func (c *rpcClient) waitLoop() {
	err := c.cmd.Wait()
	if err != nil {
		c.fail(err)
		return
	}
	c.fail(io.EOF)
}

func (c *rpcClient) fail(err error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return
	}
	c.closed = true
	c.err = err
	pending := c.pending
	c.pending = map[int64]chan rpcResult{}
	close(c.done)
	c.mu.Unlock()

	for _, ch := range pending {
		ch <- rpcResult{err: err}
	}
}

func (c *rpcClient) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

func (c *rpcClient) closeError() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return c.err
	}
	return errClientClosed
}

func (c *rpcClient) removePending(id int64) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.pending, id)
}

func (c *rpcClient) closeGracefully() error {
	_ = c.notify("shutdown", map[string]interface{}{})
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	select {
	case <-c.done:
	case <-time.After(500 * time.Millisecond):
		if c.cmd != nil && c.cmd.Process != nil {
			_ = c.cmd.Process.Signal(syscall.SIGTERM)
		}
		select {
		case <-c.done:
		case <-time.After(500 * time.Millisecond):
			if c.cmd != nil && c.cmd.Process != nil {
				_ = c.cmd.Process.Kill()
			}
		}
	}
	return nil
}
