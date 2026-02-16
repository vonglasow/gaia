package termio

import "testing"

func TestHasTTYHelpers_NoPanic(t *testing.T) {
	_ = HasTTYStdin()
	_ = HasTTYStdout()
	_ = HasPipedStdin()
}
