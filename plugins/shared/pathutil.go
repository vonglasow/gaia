package shared

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
)

// Normalize returns a clean absolute path, resolving symlinks where it can.
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
