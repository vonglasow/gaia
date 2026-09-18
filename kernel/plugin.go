package kernel

import (
	"context"

	"github.com/spf13/cobra"
)

// MCPTool describes one tool exposed via the MCP server.
type MCPTool struct {
	Name        string
	Description string
	InputSchema map[string]interface{}
	Handler     func(ctx context.Context, args map[string]interface{}) (string, error)
}

// Plugin defines a built-in plugin.
type Plugin interface {
	ID() string
	DefaultEnabled() bool
	DependsOn() []string
	ConfigSchema() []string
	Register(k *Kernel) ([]*cobra.Command, error)
	// MCPTools is what this plugin exposes over MCP, or nil.
	MCPTools() []MCPTool
}

// BasePlugin answers what most plugins answer the same way. Embed it and state
// only what is actually true of yours.
type BasePlugin struct{}

// DependsOn: no plugin needs another to be enabled first.
func (BasePlugin) DependsOn() []string { return nil }

// MCPTools: a plugin offers nothing over MCP until it says otherwise.
func (BasePlugin) MCPTools() []MCPTool { return nil }
