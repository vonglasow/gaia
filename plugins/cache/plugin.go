package cache

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"gaia/kernel"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
)

type CachePlugin struct{}

func NewCachePlugin() *CachePlugin { return &CachePlugin{} }

func (p *CachePlugin) ID() string           { return "cache" }
func (p *CachePlugin) DefaultEnabled() bool { return true }
func (p *CachePlugin) DependsOn() []string  { return nil }
func (p *CachePlugin) ConfigSchema() []string {
	return []string{
		"cache.enabled",
		"cache.dir",
		"cache.ttl_seconds",
	}
}

func (p *CachePlugin) MCPTools() []kernel.MCPTool { return nil }

func (p *CachePlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "cache",
		Short: "Manage cache entries",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List cached entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			entries, err := List()
			if err != nil {
				return err
			}
			if len(entries) == 0 {
				return shared.PrintBox(cmd.OutOrStdout(), "Cache", "No cache entries found")
			}
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].CreatedAt.After(entries[j].CreatedAt)
			})
			var b strings.Builder
			for _, entry := range entries {
				b.WriteString(fmt.Sprintf("%s\t%s\t%s\t%s\n",
					entry.Key,
					entry.PluginID,
					entry.Model,
					entry.CreatedAt.Format(time.RFC3339),
				))
				if entry.Label != "" {
					b.WriteString(fmt.Sprintf("  %s\n", entry.Label))
				}
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Cache", strings.TrimRight(b.String(), "\n"))
		},
	}

	showCmd := &cobra.Command{
		Use:   "show [key]",
		Short: "Show a cached entry",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			entry, ok, err := Get(args[0])
			if err != nil {
				return err
			}
			if !ok {
				return shared.PrintError(cmd.ErrOrStderr(), "Cache entry not found")
			}
			body := fmt.Sprintf("Key: %s\nPlugin: %s\nModel: %s\nCreated: %s\nLabel: %s\n\nResponse:\n%s",
				entry.Key,
				entry.PluginID,
				entry.Model,
				entry.CreatedAt.Format(time.RFC3339),
				entry.Label,
				entry.Response,
			)
			return shared.PrintBox(cmd.OutOrStdout(), "Cache", body)
		},
	}

	statsCmd := &cobra.Command{
		Use:   "stats",
		Short: "Show cache statistics",
		RunE: func(cmd *cobra.Command, args []string) error {
			stats, err := StatsInfo()
			if err != nil {
				return err
			}
			body := fmt.Sprintf("Entries: %d\nSize: %d bytes", stats.Count, stats.SizeBytes)
			return shared.PrintBox(cmd.OutOrStdout(), "Cache", body)
		},
	}

	clearCmd := &cobra.Command{
		Use:   "clear",
		Short: "Clear all cache entries",
		RunE: func(cmd *cobra.Command, args []string) error {
			removed, err := ClearAll()
			if err != nil {
				return err
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Cache", fmt.Sprintf("Removed %d entries", removed))
		},
	}

	deleteCmd := &cobra.Command{
		Use:   "delete [key]",
		Short: "Delete a cache entry by key",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if err := Delete(args[0]); err != nil {
				return err
			}
			return shared.PrintBox(cmd.OutOrStdout(), "Cache", fmt.Sprintf("Deleted %s", args[0]))
		},
	}

	root.AddCommand(listCmd, showCmd, statsCmd, clearCmd, deleteCmd)
	return []*cobra.Command{root}, nil
}
