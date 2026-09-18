package shared

import (
	"io"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestHasTTYHelpers_NoPanic(_ *testing.T) {
	_ = HasTTYStdin()
	_ = HasTTYStdout()
	_ = HasPipedStdin()
}

// With no terminal there is nobody to confirm anything, and a confirmer that
// blocks forever is worse than one that refuses.
func TestThereIsNoConfirmerWithoutATerminal(t *testing.T) {
	require.Nil(t, ConfirmWith(strings.NewReader(""), io.Discard),
		"tests do not run on a TTY, which is the same situation as cron or a pipe")
}
