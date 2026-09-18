package shared

import (
	"errors"
	"fmt"
	"io"
)

// ErrReported marks a failure already shown, so it exits non-zero without printing twice.
var ErrReported = errors.New("reported")

// Fail prints a reason and returns ErrReported; the two halves are not separable.
func Fail(w io.Writer, message string) error {
	_ = PrintError(w, message)
	return ErrReported
}

// Failf is Fail with formatting.
func Failf(w io.Writer, format string, args ...any) error {
	return Fail(w, fmt.Sprintf(format, args...))
}

// WasReported tells main and the harness whether to print anything themselves.
func WasReported(err error) bool {
	return errors.Is(err, ErrReported)
}
