package api

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"

	"gaia/config/roles"

	"github.com/spf13/viper"
)

var (
	cachedRoles     []roles.ResolvedRole
	cachedRolesDir  string
	cachedRolesErr  error
	cachedRolesOnce sync.Once
)

// getRolesConfig returns the roles config from viper. useYAML is true when
// roles.directory is set and should be used (caller may still fall back if LoadRoles fails).
func getRolesConfig() (cfg roles.RolesConfig, useYAML bool) {
	dir := strings.TrimSpace(viper.GetString("roles.directory"))
	if dir == "" {
		return roles.RolesConfig{}, false
	}
	cfg = roles.RolesConfig{
		Directory:    dir,
		AutoSelect:   viper.GetBool("roles.auto_select"),
		DefaultRole:  viper.GetString("roles.default_role"),
		MinThreshold: viper.GetFloat64("roles.scoring.min_threshold"),
	}
	if cfg.DefaultRole == "" {
		cfg.DefaultRole = "default"
	}
	return cfg, true
}

// getLoadedRoles loads roles from roles.directory (once per dir) and caches.
// Returns nil, nil when roles.directory is not set. Returns error when dir is set but load fails.
func getLoadedRoles() ([]roles.ResolvedRole, error) {
	cfg, useYAML := getRolesConfig()
	if !useYAML {
		clearRolesCache()
		return nil, nil
	}
	absDir, err := filepath.Abs(cfg.Directory)
	if err != nil {
		return nil, err
	}
	cachedRolesOnce.Do(func() {
		cachedRolesDir = absDir
		cachedRoles, cachedRolesErr = roles.LoadRoles(cfg.Directory)
	})
	if cachedRolesDir != absDir {
		cachedRolesOnce = sync.Once{}
		cachedRolesDir = ""
		cachedRoles = nil
		cachedRolesErr = nil
		cachedRolesOnce.Do(func() {
			cachedRolesDir = absDir
			cachedRoles, cachedRolesErr = roles.LoadRoles(cfg.Directory)
		})
	}
	return cachedRoles, cachedRolesErr
}

// clearRolesCache resets the roles cache. Called when not using YAML roles or in tests.
func clearRolesCache() {
	cachedRolesOnce = sync.Once{}
	cachedRolesDir = ""
	cachedRoles = nil
	cachedRolesErr = nil
}

// GetSystemPromptForRoleName returns the system prompt for the given role name.
// When roles.directory is set and roles are loaded, uses the role's system_prompt;
// otherwise uses viper "roles.<name>" (legacy). Template args (SHELL, GOOS) are applied.
func GetSystemPromptForRoleName(roleName string) string {
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = "sh"
	}
	roleList, err := getLoadedRoles()
	if err != nil || len(roleList) == 0 {
		tpl := viper.GetString(fmt.Sprintf("roles.%s", roleName))
		if tpl == "" {
			return ""
		}
		return fmt.Sprintf(tpl, shell, runtime.GOOS)
	}
	for _, r := range roleList {
		if r.Name == roleName {
			return roles.SystemPromptForRole(r, shell, runtime.GOOS)
		}
	}
	tpl := viper.GetString(fmt.Sprintf("roles.%s", roleName))
	if tpl == "" {
		return ""
	}
	return fmt.Sprintf(tpl, shell, runtime.GOOS)
}
