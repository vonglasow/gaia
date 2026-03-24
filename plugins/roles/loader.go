package roles

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gaia/plugins/shared"

	"gopkg.in/yaml.v3"
)

// LoadRoles loads role definitions from a directory of YAML files.
func LoadRoles(dir string) ([]Role, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("roles directory is required")
	}
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("stat roles directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("roles directory is not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read roles directory: %w", err)
	}

	var roles []Role
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("read role file %s: %w", path, err)
		}
		var role Role
		if err := yaml.Unmarshal(data, &role); err != nil {
			return nil, fmt.Errorf("parse role file %s: %w", path, err)
		}
		if role.Name == "" {
			role.Name = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		}
		if err := validateRole(role, path); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}

	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Name < roles[j].Name
	})
	return roles, nil
}

func validateRole(role Role, path string) error {
	if strings.TrimSpace(role.Name) == "" {
		return fmt.Errorf("role file %s: missing name", path)
	}
	if strings.TrimSpace(role.SystemPrompt) == "" && len(role.Providers) == 0 && len(role.Models) == 0 {
		return fmt.Errorf("role %s: missing system_prompt or provider/model overrides", role.Name)
	}
	for name := range role.Providers {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("role %s: provider override name is empty", role.Name)
		}
	}
	for name := range role.Models {
		if strings.TrimSpace(name) == "" {
			return fmt.Errorf("role %s: model override name is empty", role.Name)
		}
	}
	return nil
}

// LoadRolesFromFS allows loading from an fs.FS (used for tests).
func LoadRolesFromFS(fsys fs.FS, dir string) ([]Role, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return nil, fmt.Errorf("roles directory is required")
	}
	entries, err := fs.ReadDir(fsys, dir)
	if err != nil {
		return nil, fmt.Errorf("read roles directory: %w", err)
	}
	var roles []Role
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".yaml") && !strings.HasSuffix(name, ".yml") {
			continue
		}
		path := filepath.Join(dir, name)
		data, err := fs.ReadFile(fsys, path)
		if err != nil {
			return nil, fmt.Errorf("read role file %s: %w", path, err)
		}
		var role Role
		if err := yaml.Unmarshal(data, &role); err != nil {
			return nil, fmt.Errorf("parse role file %s: %w", path, err)
		}
		if role.Name == "" {
			role.Name = strings.TrimSuffix(strings.TrimSuffix(name, ".yaml"), ".yml")
		}
		if err := validateRole(role, path); err != nil {
			return nil, err
		}
		roles = append(roles, role)
	}
	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Name < roles[j].Name
	})
	return roles, nil
}

// DefaultRolesDir returns the default roles directory under ~/.config/gaia/roles.
func DefaultRolesDir() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "gaia", "roles"), nil
}

// EnsureRolesDir creates the roles directory if it doesn't exist.
func EnsureRolesDir(dir string) error {
	if strings.TrimSpace(dir) == "" {
		return fmt.Errorf("roles directory is required")
	}
	norm, err := shared.Normalize(dir)
	if err != nil {
		return err
	}
	return os.MkdirAll(norm, 0o755)
}
