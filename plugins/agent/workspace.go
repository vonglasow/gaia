// Package agent is gaia working on a repository: a workspace, a policy, a bounded loop.
package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Workspace is the only directory an agent may read or write.
type Workspace struct {
	root string
}

// NewWorkspace resolves symlinks, so a link out cannot pass a prefix check.
func NewWorkspace(dir string) (*Workspace, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		dir = "."
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace %q: %w", dir, err)
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace %q: %w", dir, err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("workspace %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("workspace %q is not a directory", dir)
	}
	return &Workspace{root: resolved}, nil
}

// Root is the absolute path the agent is rooted at.
func (w *Workspace) Root() string { return w.root }

// Resolve places a proposed path inside the workspace, or refuses it.
func (w *Workspace) Resolve(path string) (string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return "", fmt.Errorf("no path given")
	}
	if strings.HasPrefix(path, "~") {
		// The one spelling that deliberately means outside.
		return "", fmt.Errorf("refusing %q: paths are relative to the workspace, not to a home directory", path)
	}

	candidate := path
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(w.root, candidate)
	}
	candidate = filepath.Clean(candidate)

	// A new path resolves through its nearest existing ancestor, which must be inside.
	resolved, err := resolveExisting(candidate)
	if err != nil {
		return "", err
	}
	if !w.contains(resolved) {
		return "", fmt.Errorf("refusing %q: it is outside the workspace at %s", path, w.root)
	}
	return candidate, nil
}

// resolveExisting resolves the longest existing prefix, then reattaches the rest.
func resolveExisting(path string) (string, error) {
	remainder := ""
	current := path
	for {
		resolved, err := filepath.EvalSymlinks(current)
		if err == nil {
			if remainder == "" {
				return resolved, nil
			}
			return filepath.Join(resolved, remainder), nil
		}
		if !os.IsNotExist(err) {
			return "", fmt.Errorf("resolve %q: %w", path, err)
		}
		parent := filepath.Dir(current)
		if parent == current {
			// Nothing along the path exists, which is implausible on a live machine.
			return "", fmt.Errorf("resolve %q: no part of this path exists", path)
		}
		remainder = filepath.Join(filepath.Base(current), remainder)
		current = parent
	}
}

// contains: the separator matters, or /tmp/work-x counts as inside /tmp/work.
func (w *Workspace) contains(path string) bool {
	if path == w.root {
		return true
	}
	return strings.HasPrefix(path, w.root+string(filepath.Separator))
}

// Relative renders a path for a transcript, without the temp-directory noise.
func (w *Workspace) Relative(path string) string {
	rel, err := filepath.Rel(w.root, path)
	if err != nil {
		return path
	}
	return rel
}
