package commands_test

import (
	"io"
	"os"
	"path/filepath"
	"testing"

	"gaia/commands"
	"gaia/config"

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

func TestToolCmd_Structure(t *testing.T) {
	// Test that ToolCmd is properly defined
	assert.NotNil(t, commands.ToolCmd)
	assert.Contains(t, commands.ToolCmd.Use, "tool")
}

func TestToolCmd_NoConfig(t *testing.T) {
	// Set up config
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	// Test tool command with non-existent tool
	commands.ToolCmd.SetArgs([]string{"nonexistent", "action"})
	err = commands.ToolCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not configured")
}
