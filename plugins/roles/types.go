package roles

type ProviderOverride struct {
	SystemPrompt string `yaml:"system_prompt"`
}

type ModelOverride struct {
	SystemPrompt string `yaml:"system_prompt"`
}

// Role defines a role loaded from YAML.
type Role struct {
	Name         string                      `yaml:"name"`
	Description  string                      `yaml:"description,omitempty"`
	Priority     int                         `yaml:"priority,omitempty"`
	Exclusive    bool                        `yaml:"exclusive,omitempty"`
	Extends      []string                    `yaml:"extends,omitempty"`
	SystemPrompt string                      `yaml:"system_prompt,omitempty"`
	Providers    map[string]ProviderOverride `yaml:"providers,omitempty"`
	Models       map[string]ModelOverride    `yaml:"models,omitempty"`
}

// ResolvedRole represents a role with inherited prompts applied.
type ResolvedRole struct {
	Name         string
	Description  string
	Priority     int
	Exclusive    bool
	SystemPrompt string
	Providers    map[string]ProviderOverride
	Models       map[string]ModelOverride
}

// RolesConfig controls role loading and auto-selection.
type RolesConfig struct {
	Directory    string
	AutoSelect   bool
	DefaultRole  string
	MinThreshold float64
}

// SelectionResult captures auto-role selection details.
type SelectionResult struct {
	RoleName    string
	Score       float64
	Threshold   float64
	Matched     bool
	Reason      string
	AllScores   map[string]float64
	SortedRoles []string
}
