package pathutil

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNormalize_RelativePath(t *testing.T) {
	got, err := Normalize(".")
	if err != nil {
		t.Fatalf("Normalize(.): %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("Normalize(.) should return absolute path, got %q", got)
	}
	if got != filepath.Clean(got) {
		t.Errorf("Normalize(.) should return clean path, got %q", got)
	}
}

func TestNormalize_NonExistentPath(t *testing.T) {
	dir := t.TempDir()
	nonexistent := filepath.Join(dir, "does-not-exist")
	got, err := Normalize(nonexistent)
	if err != nil {
		t.Fatalf("Normalize(nonexistent): %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("Normalize(nonexistent) should return absolute path, got %q", got)
	}
	if got != filepath.Clean(got) {
		t.Errorf("Normalize(nonexistent) should return clean path, got %q", got)
	}
}

func TestNormalize_ExistingDir(t *testing.T) {
	dir := t.TempDir()
	got, err := Normalize(dir)
	if err != nil {
		t.Fatalf("Normalize(existing): %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("Normalize(existing) should return absolute path, got %q", got)
	}
	if got != filepath.Clean(got) {
		t.Errorf("Normalize(existing) should return clean path, got %q", got)
	}
}

func TestNormalize_WithSymlink(t *testing.T) {
	dir := t.TempDir()
	realPath := filepath.Join(dir, "real")
	if err := os.MkdirAll(realPath, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	linkPath := filepath.Join(dir, "link")
	if err := os.Symlink("real", linkPath); err != nil {
		t.Skip("symlink not supported or not allowed")
	}
	got, err := Normalize(linkPath)
	if err != nil {
		t.Fatalf("Normalize(symlink): %v", err)
	}
	if !filepath.IsAbs(got) {
		t.Errorf("Normalize(symlink) should return absolute path, got %q", got)
	}
	// Resolved path should point to real directory (may differ by /private on macOS)
	realResolved, err := filepath.EvalSymlinks(realPath)
	if err != nil {
		t.Fatalf("EvalSymlinks(real): %v", err)
	}
	realAbs, _ := filepath.Abs(realResolved)
	if got != filepath.Clean(realAbs) {
		// macOS may return /private/var/... vs /var/...
		gotNorm, _ := filepath.EvalSymlinks(got)
		wantNorm, _ := filepath.EvalSymlinks(realAbs)
		if gotNorm != wantNorm {
			t.Errorf("Normalize(symlink) should resolve to real path: got %q, want %q", got, filepath.Clean(realAbs))
		}
	}
}
