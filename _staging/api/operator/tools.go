package operator

import (
	"context"
	"sync"
)

const (
	// RunCmdName is the name of the built-in shell command tool.
	RunCmdName = "run_cmd"
)

// Tool represents a callable tool (e.g. run_cmd) with name, description, risk level, schema, and executor.
type Tool struct {
	Name        string
	Description string
	RiskLevel   RiskLevel
	Schema      map[string]string // e.g. {"cmd": "shell command to run"}
	Exec        func(ctx context.Context, args map[string]string) (stdout, stderr string, err error)
}

// Registry holds tools by name.
type Registry struct {
	mu    sync.RWMutex
	tools map[string]*Tool
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{tools: make(map[string]*Tool)}
}

// Register adds a tool. It overwrites if the name already exists.
func (r *Registry) Register(t *Tool) {
	if t == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.tools[t.Name] = t
}

// Get returns the tool by name, or nil if not found.
func (r *Registry) Get(name string) *Tool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.tools[name]
}

// List returns a copy of all registered tool names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.tools))
	for n := range r.tools {
		names = append(names, n)
	}
	return names
}

// DefaultToolRegistry returns a registry with only run_cmd registered.
// The Exec for run_cmd is set by the executor package using a ShellRunner;
// callers should use executor.NewExecutor(registry, shellRunner) to wire it.
func DefaultToolRegistry(shellRunner ShellRunner) *Registry {
	r := NewRegistry()
	r.Register(&Tool{
		Name:        RunCmdName,
		Description: "Execute a shell command. Use for reading system state (e.g. df, du, find).",
		RiskLevel:   RiskMedium,
		Schema:      map[string]string{"cmd": "shell command to run"},
		Exec: func(ctx context.Context, args map[string]string) (stdout, stderr string, err error) {
			if shellRunner == nil {
				return "", "", nil
			}
			return shellRunner.Run(ctx, args["cmd"])
		},
	})
	return r
}

// ShellRunner runs a shell command with context (e.g. for timeout).
// Implemented by commands package using ExecuteExternalCommandWithContext.
type ShellRunner interface {
	Run(ctx context.Context, cmd string) (stdout, stderr string, err error)
}
