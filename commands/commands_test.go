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

func TestConfigCmd_List(t *testing.T) {
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	commands.RootCmd.SetArgs([]string{"config", "list"})
	err = commands.RootCmd.Execute()
	require.NoError(t, err)
}

func TestConfigCmd_GetSet(t *testing.T) {
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	// Test set
	commands.RootCmd.SetArgs([]string{"config", "set", "model", "test-model"})
	err = commands.RootCmd.Execute()
	require.NoError(t, err)

	// Test get
	commands.RootCmd.SetArgs([]string{"config", "get", "model"})
	err = commands.RootCmd.Execute()
	require.NoError(t, err)
}

func TestConfigCmd_GetNonExistent(t *testing.T) {
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	commands.GetCmd.SetArgs([]string{"nonexistent.key"})
	err = commands.GetCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "is not set")
}

func TestConfigCmd_SetInvalidKey(t *testing.T) {
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	commands.SetCmd.SetArgs([]string{"invalid.key", "value"})
	err = commands.SetCmd.Execute()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid config key")
}

func TestConfigCmd_Path(t *testing.T) {
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	// Capture output
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	commands.PathCmd.SetArgs([]string{})
	err = commands.PathCmd.Execute()
	require.NoError(t, err)

	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe: %v", err)
	}
	os.Stdout = oldStdout

	out, _ := io.ReadAll(r)
	assert.Contains(t, string(out), "config.yaml")
}

func TestCacheCmd_Stats(t *testing.T) {
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	commands.RootCmd.SetArgs([]string{"cache", "stats"})
	err = commands.RootCmd.Execute()
	require.NoError(t, err)
}

func TestCacheCmd_List(t *testing.T) {
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	commands.RootCmd.SetArgs([]string{"cache", "list"})
	err = commands.RootCmd.Execute()
	require.NoError(t, err)
}

func TestCacheCmd_Clear(t *testing.T) {
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	commands.RootCmd.SetArgs([]string{"cache", "clear"})
	err = commands.RootCmd.Execute()
	require.NoError(t, err)
}

func TestCacheCmd_Dump(t *testing.T) {
	tmpDir := t.TempDir()
	config.CfgFile = filepath.Join(tmpDir, "config.yaml")
	err := config.InitConfig()
	require.NoError(t, err)

	commands.RootCmd.SetArgs([]string{"cache", "dump"})
	err = commands.RootCmd.Execute()
	require.NoError(t, err)
}

func TestReadStdin_NoInput(t *testing.T) {
	originalStdin := os.Stdin
	defer func() { os.Stdin = originalStdin }()

	// Set stdin to a regular file (CharDevice mode)
	tmpFile := filepath.Join(t.TempDir(), "test.txt")
	f, err := os.Create(tmpFile)
	require.NoError(t, err)
	defer func() {
		if err := f.Close(); err != nil {
			t.Logf("failed to close file: %v", err)
		}
	}()

	os.Stdin = f
	out := commands.CallReadStdinForTest()
	assert.Equal(t, "", out)
}

func TestReadStdin_EmptyPipe(t *testing.T) {
	originalStdin := os.Stdin
	defer func() { os.Stdin = originalStdin }()

	r, w, _ := os.Pipe()
	os.Stdin = r
	if err := w.Close(); err != nil {
		t.Fatalf("failed to close pipe: %v", err)
	}

	out := commands.CallReadStdinForTest()
	assert.Equal(t, "", out)
}
