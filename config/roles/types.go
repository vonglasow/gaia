package roles

// Role represents a parsed role from YAML.
type Role struct {
	Name         string         `yaml:"name"`
	Description  string         `yaml:"description"`
	Priority     int            `yaml:"priority"`
	Enabled      *bool          `yaml:"enabled"` // nil = not set, treated as true
	SystemPrompt string         `yaml:"system_prompt"`
	Matching     MatchingConfig `yaml:"matching"`
}

// MatchingConfig holds threshold, imports, and signals for role matching.
type MatchingConfig struct {
	Threshold       float64  `yaml:"threshold"`
	Imports         []string `yaml:"imports"`
	Signals         []Signal `yaml:"signals"`
	NegativeSignals []Signal `yaml:"negative_signals"`
}

// Signal defines a single matching signal (keyword or regex).
type Signal struct {
	Type   string   `yaml:"type"` // "keyword" or "regex"
	Values []string `yaml:"values"`
	Weight float64  `yaml:"weight"`
}

// SignalGroupFile is the structure of a file in _signals/.
type SignalGroupFile struct {
	Group  string   `yaml:"group"`
	Type   string   `yaml:"type"`
	Values []string `yaml:"values"`
}

// RolesConfig is the in-memory config for role selection (from main config).
type RolesConfig struct {
	Directory    string
	AutoSelect   bool
	DefaultRole  string
	MinThreshold float64
}
