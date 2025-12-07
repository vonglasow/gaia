package commands_test

import (
	"io"
	"os"
	"testing"

	"gaia/commands"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadStdin(t *testing.T) {
	originalStdin := os.Stdin
	defer func() { os.Stdin = originalStdin }()

	r, w, _ := os.Pipe()
	os.Stdin = r

	input := "hello stdin"
	_, _ = w.Write([]byte(input))
	if err := w.Close(); err != nil { // errcheck compliant
		t.Fatalf("failed to close pipe: %v", err)
	}

	out := commands.CallReadStdinForTest()
	if out != input {
		t.Fatalf("expected %q got %q", input, out)
	}
}

func TestVersionCmd(t *testing.T) {
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	_, err := commands.VersionCmd.ExecuteC()
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe: %v", err)
	}
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)

	require.NoError(t, err)
	assert.Contains(t, string(out), "Gaia")
}
