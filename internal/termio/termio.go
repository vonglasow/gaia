package termio

import (
	"os"

	"github.com/mattn/go-isatty"
)

// HasTTYStdin reports whether stdin is connected to a terminal.
func HasTTYStdin() bool {
	fd := os.Stdin.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// HasTTYStdout reports whether stdout is connected to a terminal.
func HasTTYStdout() bool {
	fd := os.Stdout.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

// HasPipedStdin reports whether stdin carries piped or redirected input.
func HasPipedStdin() bool {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false
	}
	return (stat.Mode() & os.ModeCharDevice) == 0
}
