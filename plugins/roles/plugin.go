package roles

import (
	"fmt"
	"sort"
	"strings"

	"gaia/kernel"
	"gaia/plugins/shared"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

type RolesPlugin struct{}

func NewRolesPlugin() *RolesPlugin { return &RolesPlugin{} }

func (p *RolesPlugin) ID() string           { return "roles" }
func (p *RolesPlugin) DefaultEnabled() bool { return true }
func (p *RolesPlugin) DependsOn() []string  { return nil }
func (p *RolesPlugin) ConfigSchema() []string {
	return []string{
		"roles.directory",
		"roles.auto_select",
		"roles.default_role",
		"roles.scoring.min_threshold",
		"roles.scoring.weight",
		"roles.debug",
		"roles.keywords.*",
	}
}

func (p *RolesPlugin) Register(k *kernel.Kernel) ([]*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "roles",
		Short: "Manage roles",
	}

	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List available roles",
		RunE: func(cmd *cobra.Command, args []string) error {
			roles, err := loadRolesWithDefaults()
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			if len(roles) == 0 {
				return shared.PrintBox(cmd.OutOrStdout(), "Roles", "No roles found")
			}
			names := make([]string, 0, len(roles))
			for _, r := range roles {
				names = append(names, r.Name)
			}
			sort.Strings(names)
			return shared.PrintBox(cmd.OutOrStdout(), "Roles", strings.Join(names, "\n"))
		},
	}

	showCmd := &cobra.Command{
		Use:   "show [name]",
		Short: "Show a role's prompt",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			rolesList, err := loadRolesWithDefaults()
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			if len(rolesList) == 0 {
				return shared.PrintError(cmd.ErrOrStderr(), "No roles found")
			}
			resolved, err := ResolveInheritance(rolesList)
			if err != nil {
				return shared.PrintError(cmd.ErrOrStderr(), err.Error())
			}
			name := args[0]
			role, ok := resolved[name]
			if !ok {
				return shared.PrintError(cmd.ErrOrStderr(), fmt.Sprintf("Role %q not found", name))
			}
			body := fmt.Sprintf("Name: %s\nPriority: %d\nExclusive: %v\n\n%s",
				role.Name, role.Priority, role.Exclusive, role.SystemPrompt)
			return shared.PrintBox(cmd.OutOrStdout(), "Role", body)
		},
	}

	resolveCmd := &cobra.Command{
		Use:   "resolve [text]",
		Short: "Resolve auto-role for the given text",
		Args:  cobra.MinimumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if viper.GetBool("roles.debug") {
				SetDebugWriter(cmd.ErrOrStderr())
			} else {
				SetDebugWriter(nil)
			}
			input := strings.Join(args, " ")
			keywords := loadKeywordConfig()
			weight := viper.GetFloat64("roles.scoring.weight")
			if weight == 0 {
				weight = 1.0
			}
			scores := ScoreText(input, BuildScoringFromConfig(keywords, weight))
			threshold := viper.GetFloat64("roles.scoring.min_threshold")
			result := SelectRole(scores, threshold, viper.GetString("roles.default_role"))
			LogScores(scores, threshold, result.RoleName)
			body := fmt.Sprintf("Role: %s\nScore: %.2f\nMatched: %v\nReason: %s",
				result.RoleName, result.Score, result.Matched, result.Reason)
			return shared.PrintBox(cmd.OutOrStdout(), "Resolve", body)
		},
	}

	root.AddCommand(listCmd, showCmd, resolveCmd)
	return []*cobra.Command{root}, nil
}

func loadRolesWithDefaults() ([]Role, error) {
	dir := strings.TrimSpace(viper.GetString("roles.directory"))
	if dir == "" {
		defaultDir, err := DefaultRolesDir()
		if err != nil {
			return nil, err
		}
		dir = defaultDir
	}
	if err := EnsureRolesDir(dir); err != nil {
		return nil, err
	}
	return LoadRoles(dir)
}

func loadKeywordConfig() map[string][]string {
	out := map[string][]string{}
	for _, key := range viper.AllKeys() {
		if !strings.HasPrefix(key, "roles.keywords.") {
			continue
		}
		role := strings.TrimPrefix(key, "roles.keywords.")
		if role == "" {
			continue
		}
		out[role] = viper.GetStringSlice(key)
	}
	return out
}
