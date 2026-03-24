package kernel

import "github.com/spf13/cobra"

// Plugin defines a built-in plugin.
type Plugin interface {
	ID() string
	DefaultEnabled() bool
	DependsOn() []string
	ConfigSchema() []string
	Register(k *Kernel) ([]*cobra.Command, error)
}
