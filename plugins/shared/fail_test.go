package shared

import (
	"bytes"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/require"
)

// Fail exists because of one mistake.

func TestFailShowsTheReasonAndReportsAFailure(t *testing.T) {
	var out bytes.Buffer

	err := Fail(&out, "something went wrong")

	require.ErrorIs(t, err, ErrReported, "the shell must see a failure")
	require.Contains(t, out.String(), "something went wrong", "and the person must see why")
}

func TestFailfNamesWhatWentWrong(t *testing.T) {
	var out bytes.Buffer

	err := Failf(&out, "cannot read %s: %v", "config.yaml", errors.New("no such file"))

	require.ErrorIs(t, err, ErrReported)
	require.Contains(t, out.String(), "cannot read config.yaml: no such file")
}

// Even with nothing to print.
func TestFailWithNothingToSayStillFails(t *testing.T) {
	var out bytes.Buffer

	err := Fail(&out, "   ")

	require.ErrorIs(t, err, ErrReported)
	require.Empty(t, out.String())
}

// A writer that cannot be written to is a closed pipe.
func TestFailStillFailsWhenTheMessageCannotBeWritten(t *testing.T) {
	require.ErrorIs(t, Fail(failingWriter{}, "something went wrong"), ErrReported)
}

func TestWasReportedRecognisesOnlyWhatWasReported(t *testing.T) {
	var out bytes.Buffer

	require.True(t, WasReported(Fail(&out, "shown")))
	require.False(t, WasReported(nil))
	require.False(t, WasReported(errors.New("never shown")))
}

// Wrapping is how a caller adds context on the way up.
func TestWasReportedSeesThroughWrapping(t *testing.T) {
	var out bytes.Buffer
	wrapped := fmt.Errorf("while loading config: %w", Fail(&out, "shown"))

	require.True(t, WasReported(wrapped))
}
