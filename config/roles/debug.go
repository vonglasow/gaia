package roles

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/spf13/viper"
)

const debugPrefix = "[ROLES DEBUG] "

var debugWriter io.Writer = os.Stderr

// SetDebugWriter sets the writer for debug output (for tests).
func SetDebugWriter(w io.Writer) {
	debugWriter = w
}

// IsRolesDebug returns true when roles debug output is enabled (config or CLI).
func IsRolesDebug() bool {
	return viper.GetBool("roles.debug")
}

func debugf(format string, args ...interface{}) {
	if !IsRolesDebug() {
		return
	}
	_, _ = fmt.Fprintf(debugWriter, debugPrefix+format+"\n", args...)
}

// LogLoadedRoles logs the list of loaded role names (deterministic order).
func LogLoadedRoles(resolved map[string]ResolvedRole) {
	if len(resolved) == 0 {
		return
	}
	names := make([]string, 0, len(resolved))
	for name := range resolved {
		names = append(names, name)
	}
	sort.Strings(names)
	debugf("Loaded roles: %s", strings.Join(names, ", "))
}

// LogInheritanceTree logs which role extends which (from raw Role map).
func LogInheritanceTree(roles map[string]Role) {
	if roles == nil {
		return
	}
	names := make([]string, 0, len(roles))
	for name := range roles {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		r := roles[name]
		if len(r.Extends) > 0 {
			debugf("Role %s extends: %s", name, strings.Join(r.Extends, ", "))
		}
	}
}

// LogScore logs one role's score and threshold result.
func LogScore(roleName string, score float64, threshold float64, pass bool) {
	result := "FAIL"
	if pass {
		result = "PASS"
	}
	debugf("Score(%s)=%.2f threshold=%.2f %s", roleName, score, threshold, result)
}

// LogFinalSelection logs the selected role and optional ordering.
func LogFinalSelection(selected ResolvedRole, allNames []string) {
	debugf("Final roles: %s", selected.Name)
	if len(allNames) > 1 {
		debugf("Final order after sorting: %s", strings.Join(allNames, ", "))
	}
}

// LogOverrideExclusive logs override/exclusive resolution for a role (optional detail).
func LogOverrideExclusive(roleName string, exclusive bool, priority int) {
	debugf("Role %s: exclusive=%v priority=%d", roleName, exclusive, priority)
}

// LogSystemPromptPreview logs a short preview of the composed system prompt.
func LogSystemPromptPreview(roleName string, prompt string, maxLen int) {
	if prompt == "" {
		debugf("System prompt (%s): (empty)", roleName)
		return
	}
	preview := prompt
	if len(preview) > maxLen {
		preview = preview[:maxLen] + "..."
	}
	preview = strings.ReplaceAll(preview, "\n", " ")
	debugf("System prompt (%s) preview: %s", roleName, preview)
}
