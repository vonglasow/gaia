package pathutil

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Normalize resolves to an absolute, cleaned path and resolves symlinks when possible.
// If the path does not exist yet, it still returns a clean absolute path.
func Normalize(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(abs)
	if err == nil {
		return filepath.Clean(resolved), nil
	}
	if errors.Is(err, os.ErrNotExist) || errors.Is(err, fs.ErrNotExist) {
		return filepath.Clean(abs), nil
	}
	return "", err
}
