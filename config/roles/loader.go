package roles

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const signalsDirName = "_signals"

// LoadRoles loads all role YAML files from dir (excluding _signals), resolves
// signal imports from _signals, and returns roles sorted by priority (higher first).
// Directory structure: dir/*.yaml = roles, dir/_signals/*.yaml = signal groups.
func LoadRoles(dir string) ([]Role, error) {
	if dir == "" {
		return nil, fmt.Errorf("roles directory is empty")
	}
	absDir, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve roles directory: %w", err)
	}
	finfo, err := os.Stat(absDir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("roles directory does not exist: %s", absDir)
		}
		return nil, fmt.Errorf("roles directory: %w", err)
	}
	if !finfo.IsDir() {
		return nil, fmt.Errorf("roles path is not a directory: %s", absDir)
	}

	groups, err := loadSignalGroups(filepath.Join(absDir, signalsDirName))
	if err != nil {
		return nil, err
	}

	entries, err := os.ReadDir(absDir)
	if err != nil {
		return nil, fmt.Errorf("read roles directory: %w", err)
	}

	var roles []Role
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".yaml") {
			continue
		}
		p := filepath.Join(absDir, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read role file %s: %w", e.Name(), err)
		}
		var r Role
		if err := yaml.Unmarshal(data, &r); err != nil {
			return nil, fmt.Errorf("parse role file %s: %w", e.Name(), err)
		}
		if r.Name == "" {
			r.Name = strings.TrimSuffix(strings.TrimSuffix(e.Name(), ".yaml"), ".yml")
		}
		// enabled: omit in YAML → nil → treat as true
		if r.Enabled == nil {
			t := true
			r.Enabled = &t
		}
		if !*r.Enabled {
			continue
		}
		if err := resolveImports(&r, groups); err != nil {
			return nil, fmt.Errorf("role %s: %w", r.Name, err)
		}
		roles = append(roles, r)
	}

	sort.Slice(roles, func(i, j int) bool {
		return roles[i].Priority > roles[j].Priority
	})
	return roles, nil
}

// loadSignalGroups reads _signals/*.yaml and returns a map: group name -> signals.
func loadSignalGroups(signalsPath string) (map[string][]Signal, error) {
	groups := make(map[string][]Signal)
	finfo, err := os.Stat(signalsPath)
	if err != nil {
		if os.IsNotExist(err) {
			return groups, nil
		}
		return nil, fmt.Errorf("signals directory: %w", err)
	}
	if !finfo.IsDir() {
		return groups, nil
	}

	entries, err := os.ReadDir(signalsPath)
	if err != nil {
		return nil, fmt.Errorf("read _signals: %w", err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(strings.ToLower(e.Name()), ".yaml") {
			continue
		}
		p := filepath.Join(signalsPath, e.Name())
		data, err := os.ReadFile(p)
		if err != nil {
			return nil, fmt.Errorf("read signal file %s: %w", e.Name(), err)
		}
		var f SignalGroupFile
		if err := yaml.Unmarshal(data, &f); err != nil {
			return nil, fmt.Errorf("parse signal file %s: %w", e.Name(), err)
		}
		if f.Group == "" {
			continue
		}
		if f.Type == "" {
			f.Type = "keyword"
		}
		sig := Signal{Type: f.Type, Values: f.Values, Weight: 1.0}
		if len(sig.Values) == 0 {
			continue
		}
		groups[f.Group] = append(groups[f.Group], sig)
	}
	return groups, nil
}

// resolveImports inlines imported signal groups into r.Matching.Signals (and sets default weight).
func resolveImports(r *Role, groups map[string][]Signal) error {
	var resolved []Signal
	resolved = append(resolved, r.Matching.Signals...)
	for _, imp := range r.Matching.Imports {
		g, ok := groups[imp]
		if !ok {
			return fmt.Errorf("unknown signal group %q", imp)
		}
		resolved = append(resolved, g...)
	}
	r.Matching.Signals = resolved
	return nil
}
