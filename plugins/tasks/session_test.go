package tasks

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeFakeSession(t *testing.T, dir, projectDir string, turns []map[string]interface{}) string {
	t.Helper()
	projSlug := filepath.Base(projectDir)
	projDir := filepath.Join(dir, projSlug)
	require.NoError(t, os.MkdirAll(projDir, 0755))
	f, err := os.CreateTemp(projDir, "*.jsonl")
	require.NoError(t, err)
	defer f.Close()
	for _, turn := range turns {
		b, _ := json.Marshal(turn)
		_, _ = f.Write(append(b, '\n'))
	}
	return f.Name()
}

func TestReadSessions_ParsesUserMessages(t *testing.T) {
	dir := t.TempDir()
	ts := time.Now().UTC()
	writeFakeSession(t, dir, "proj-a", []map[string]interface{}{
		{"type": "user", "timestamp": ts.Format(time.RFC3339), "message": map[string]interface{}{
			"role": "user", "content": "Fix the CI tests for Renovate",
		}},
		{"type": "assistant", "timestamp": ts.Add(time.Minute).Format(time.RFC3339), "message": map[string]interface{}{
			"role": "assistant", "content": "Let me check the CI config",
		}},
		{"type": "user", "timestamp": ts.Add(2 * time.Minute).Format(time.RFC3339), "message": map[string]interface{}{
			"role": "user", "content": "Looks good, commit it",
		}},
	})

	sessions, err := ReadSessions(dir, time.Time{})
	require.NoError(t, err)
	require.Len(t, sessions, 1)

	s := sessions[0]
	assert.Equal(t, ts.Format("2006-01-02"), s.Date)
	assert.GreaterOrEqual(t, s.DurationMinutes, 2)
	assert.Len(t, s.Messages, 2) // only user messages
	assert.Contains(t, s.Messages[0], "CI tests")
}

func TestReadSessions_Since_FiltersOldSessions(t *testing.T) {
	dir := t.TempDir()
	old := time.Now().Add(-48 * time.Hour).UTC()
	recent := time.Now().Add(-1 * time.Hour).UTC()
	cutoff := time.Now().Add(-24 * time.Hour)

	writeFakeSession(t, dir, "proj-a", []map[string]interface{}{
		{"type": "user", "timestamp": old.Format(time.RFC3339), "message": map[string]interface{}{
			"role": "user", "content": "old session",
		}},
	})
	writeFakeSession(t, dir, "proj-a", []map[string]interface{}{
		{"type": "user", "timestamp": recent.Format(time.RFC3339), "message": map[string]interface{}{
			"role": "user", "content": "recent session",
		}},
	})

	sessions, err := ReadSessions(dir, cutoff)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Contains(t, sessions[0].Messages[0], "recent session")
}

func TestReadSessions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	sessions, err := ReadSessions(dir, time.Time{})
	require.NoError(t, err)
	assert.Len(t, sessions, 0)
}

func TestReadSessions_SkipsNonUserMessages(t *testing.T) {
	dir := t.TempDir()
	ts := time.Now().UTC()
	writeFakeSession(t, dir, "proj-a", []map[string]interface{}{
		{"type": "mode", "timestamp": ts.Format(time.RFC3339)},
		{"type": "user", "timestamp": ts.Format(time.RFC3339), "message": map[string]interface{}{
			"role": "user", "content": "user message",
		}},
		{"type": "assistant", "timestamp": ts.Add(time.Minute).Format(time.RFC3339), "message": map[string]interface{}{
			"role": "assistant", "content": "assistant reply",
		}},
	})

	sessions, err := ReadSessions(dir, time.Time{})
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Len(t, sessions[0].Messages, 1) // only user messages
	assert.Equal(t, "user message", sessions[0].Messages[0])
}

func TestReadSessions_ContentArray(t *testing.T) {
	dir := t.TempDir()
	ts := time.Now().UTC()
	// content can be an array of {type,text} objects
	writeFakeSession(t, dir, "proj-a", []map[string]interface{}{
		{"type": "user", "timestamp": ts.Format(time.RFC3339), "message": map[string]interface{}{
			"role": "user",
			"content": []interface{}{
				map[string]interface{}{"type": "text", "text": "part one"},
				map[string]interface{}{"type": "text", "text": " part two"},
			},
		}},
		{"type": "assistant", "timestamp": ts.Add(time.Minute).Format(time.RFC3339)},
	})

	sessions, err := ReadSessions(dir, time.Time{})
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Contains(t, sessions[0].Messages[0], "part one")
}

func TestClaudeProjectsDir(t *testing.T) {
	dir := ClaudeProjectsDir()
	assert.Contains(t, dir, ".claude")
	assert.Contains(t, dir, "projects")
}
