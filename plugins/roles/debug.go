package roles

import (
	"fmt"
	"io"
	"sort"
	"strings"
)

const debugPrefix = "[ROLES DEBUG] "

// DebugWriter is used for debug output.
var DebugWriter io.Writer

// SetDebugWriter sets the writer for debug output.
func SetDebugWriter(w io.Writer) {
	DebugWriter = w
}

func debugf(format string, args ...interface{}) {
	if DebugWriter == nil {
		return
	}
	_, _ = fmt.Fprintf(DebugWriter, debugPrefix+format+"\n", args...)
}

// LogScores logs score details for auto-role selection.
func LogScores(scores map[string]float64, threshold float64, selected string) {
	if DebugWriter == nil {
		return
	}
	names := make([]string, 0, len(scores))
	for name := range scores {
		names = append(names, name)
	}
	sort.Strings(names)
	parts := make([]string, 0, len(names))
	for _, name := range names {
		parts = append(parts, fmt.Sprintf("%s=%.2f", name, scores[name]))
	}
	debugf("Scores: %s", strings.Join(parts, ", "))
	debugf("Threshold: %.2f selected=%s", threshold, selected)
}
