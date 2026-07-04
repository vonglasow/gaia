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
	// MCPTools returns the list of tools this plugin exposes via the MCP server.
	// Return nil if the plugin has no MCP tools.
	MCPTools() []MCPTool
}
