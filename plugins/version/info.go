package version

import (
	"fmt"
	"runtime/debug"
	"strings"
	"time"
)

var (
	Version = "dev"
	Commit  = ""
	Date    = ""
)

// Info returns a human-friendly version string.
func Info() string {
	version := firstNonEmpty(Version, "dev")
	commit := firstNonEmpty(Commit, readBuildSetting("vcs.revision"), "unknown")
	date := firstNonEmpty(Date, readBuildSetting("vcs.time"), "unknown")

	if t, err := time.Parse(time.RFC3339, date); err == nil {
		date = t.UTC().Format(time.RFC3339)
	}

	if commit != "unknown" && len(commit) > 12 {
		commit = commit[:12]
	}

	return fmt.Sprintf("Gaia %s, commit %s, built at %s", version, commit, date)
}

func readBuildSetting(key string) string {
	buildInfo, ok := debug.ReadBuildInfo()
	if !ok {
		return ""
	}
	for _, setting := range buildInfo.Settings {
		if setting.Key == key {
			return strings.TrimSpace(setting.Value)
		}
	}
	return ""
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
