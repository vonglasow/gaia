package version

import (
	"gaia/kernel"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
)

type VersionPlugin struct{}

func NewVersionPlugin() *VersionPlugin { return &VersionPlugin{} }

func (p *VersionPlugin) ID() string           { return "version" }
func (p *VersionPlugin) DefaultEnabled() bool { return true }
func (p *VersionPlugin) DependsOn() []string  { return nil }
func (p *VersionPlugin) ConfigSchema() []string {
	return nil
}

func (p *VersionPlugin) MCPTools() []kernel.MCPTool { return nil }

func (p *VersionPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			return shared.PrintBox(cmd.OutOrStdout(), "Version", Info())
		},
	}
	return []*cobra.Command{cmd}, nil
}
