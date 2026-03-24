package sanitize

import (
	"gaia/kernel"

	"github.com/spf13/cobra"
)

// Sanitizer plugin registers config keys for sanitization. It exposes no commands.
type SanitizerPlugin struct{}

func NewSanitizerPlugin() *SanitizerPlugin { return &SanitizerPlugin{} }

func (p *SanitizerPlugin) ID() string           { return "sanitize" }
func (p *SanitizerPlugin) DefaultEnabled() bool { return true }
func (p *SanitizerPlugin) DependsOn() []string  { return nil }
func (p *SanitizerPlugin) ConfigSchema() []string {
	return []string{
		"sanitize.enabled",
		"sanitize.level",
		"sanitize.max_tokens_after",
		"sanitize.log_stats",
	}
}

func (p *SanitizerPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	return []*cobra.Command{}, nil
}
