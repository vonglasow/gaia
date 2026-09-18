package agent

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// A model asked to "fix the config" will propose ~/.ssh/config without malice.

func aWorkspace(t *testing.T) (*Workspace, string) {
	t.Helper()
	dir := t.TempDir()
	// Resolved because macOS temporary directories are symlinks.
	resolved, err := filepath.EvalSymlinks(dir)
	require.NoError(t, err)

	ws, err := NewWorkspace(resolved)
	require.NoError(t, err)
	return ws, resolved
}

func TestAPathInsideResolves(t *testing.T) {
	ws, root := aWorkspace(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o600))

	resolved, err := ws.Resolve("main.go")

	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "main.go"), resolved)
}

func TestANestedPathResolves(t *testing.T) {
	ws, root := aWorkspace(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "plugins", "ask"), 0o755))

	resolved, err := ws.Resolve("plugins/ask/plugin.go")

	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "plugins", "ask", "plugin.go"), resolved)
}

// A tool that creates files needs a path that does not exist yet to resolve.
func TestAFileThatDoesNotExistYetStillResolves(t *testing.T) {
	ws, root := aWorkspace(t)

	resolved, err := ws.Resolve("new-file.go")

	require.NoError(t, err)
	require.Equal(t, filepath.Join(root, "new-file.go"), resolved)
}

func TestClimbingOutIsRefused(t *testing.T) {
	ws, _ := aWorkspace(t)

	for _, path := range []string{
		"../outside.go",
		"../../etc/passwd",
		"plugins/../../outside.go",
		"/etc/passwd",
	} {
		t.Run(path, func(t *testing.T) {
			_, err := ws.Resolve(path)
			require.ErrorContains(t, err, "outside the workspace")
		})
	}
}

// The case a prefix check alone gets wrong.
func TestClimbingOutThroughAPathThatDoesNotExistIsRefused(t *testing.T) {
	ws, _ := aWorkspace(t)

	_, err := ws.Resolve("nowhere/../../../etc/hosts")

	require.Error(t, err)
}

// A symlink inside the workspace pointing out of it passes every prefix check ever.
func TestASymlinkOutOfTheWorkspaceIsRefused(t *testing.T) {
	ws, root := aWorkspace(t)
	outside := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(outside, "secret"), []byte("secret"), 0o600))
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "escape")))

	_, err := ws.Resolve("escape/secret")

	require.ErrorContains(t, err, "outside the workspace")
}

func TestASymlinkInsideTheWorkspaceIsFine(t *testing.T) {
	ws, root := aWorkspace(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "resolved"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "resolved", "file.go"), []byte("x"), 0o600))
	require.NoError(t, os.Symlink(filepath.Join(root, "resolved"), filepath.Join(root, "link")))

	_, err := ws.Resolve("link/file.go")

	require.NoError(t, err)
}

// Expanding ~ would be the one case where a path deliberately means the home directory.
func TestAHomeRelativePathIsRefused(t *testing.T) {
	ws, _ := aWorkspace(t)

	_, err := ws.Resolve("~/.ssh/config")

	require.ErrorContains(t, err, "home directory")
}

func TestAnEmptyPathIsRefused(t *testing.T) {
	ws, _ := aWorkspace(t)

	_, err := ws.Resolve("   ")

	require.ErrorContains(t, err, "no path")
}

// A directory that is a sibling with a longer name is not inside.
func TestASiblingWithALongerNameIsOutside(t *testing.T) {
	parent := t.TempDir()
	resolved, err := filepath.EvalSymlinks(parent)
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(resolved, "work"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(resolved, "work-elsewhere"), 0o755))

	ws, err := NewWorkspace(filepath.Join(resolved, "work"))
	require.NoError(t, err)

	require.False(t, ws.contains(filepath.Join(resolved, "work-elsewhere")))
	require.True(t, ws.contains(filepath.Join(resolved, "work", "inside")))
	require.True(t, ws.contains(ws.Root()))
}

func TestTheWorkspaceMustBeADirectoryThatExists(t *testing.T) {
	dir := t.TempDir()
	file := filepath.Join(dir, "a-file")
	require.NoError(t, os.WriteFile(file, []byte("x"), 0o600))

	_, err := NewWorkspace(file)
	require.ErrorContains(t, err, "not a directory")

	_, err = NewWorkspace(filepath.Join(dir, "no-such-directory"))
	require.Error(t, err)
}

func TestAnEmptyWorkspaceMeansHere(t *testing.T) {
	ws, err := NewWorkspace("  ")

	require.NoError(t, err)
	require.NotEmpty(t, ws.Root())
}

// A transcript full of temp-directory noise tells nobody which file changed.
func TestPathsAreReportedRelativeToTheWorkspace(t *testing.T) {
	ws, root := aWorkspace(t)

	require.Equal(t, "main.go", ws.Relative(filepath.Join(root, "main.go")))
	require.Equal(t, filepath.Join("plugins", "ask", "plugin.go"),
		ws.Relative(filepath.Join(root, "plugins", "ask", "plugin.go")))
}
