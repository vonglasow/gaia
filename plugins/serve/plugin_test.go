package serve

import (
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConfigDir(t *testing.T) {
	dir := configDir()
	home, _ := os.UserHomeDir()
	assert.Equal(t, filepath.Join(home, ".config", "gaia"), dir)
}

func TestPidPath(t *testing.T) {
	p := pidPath()
	assert.True(t, filepath.IsAbs(p))
	assert.Equal(t, "serve.pid", filepath.Base(p))
}

func TestLogPath(t *testing.T) {
	p := logPath()
	assert.True(t, filepath.IsAbs(p))
	assert.Equal(t, "serve.log", filepath.Base(p))
}

func TestReadPID_NotFound(t *testing.T) {
	_, err := readPID(filepath.Join(t.TempDir(), "missing.pid"))
	assert.Error(t, err)
}

func TestReadPID_Valid(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.pid")
	require.NoError(t, os.WriteFile(f, []byte("12345\n"), 0644))
	pid, err := readPID(f)
	require.NoError(t, err)
	assert.Equal(t, 12345, pid)
}

func TestReadPID_Invalid(t *testing.T) {
	f := filepath.Join(t.TempDir(), "test.pid")
	require.NoError(t, os.WriteFile(f, []byte("not-a-number"), 0644))
	_, err := readPID(f)
	assert.Error(t, err)
}

func TestIsRunning_OwnPID(t *testing.T) {
	assert.True(t, isRunning(os.Getpid()))
}

func TestIsRunning_InvalidPID(t *testing.T) {
	assert.False(t, isRunning(-1))
}

func TestIsRunning_DeadPID(t *testing.T) {
	// PID 999999999 is almost certainly not running.
	assert.False(t, isRunning(999999999))
}

func TestServePlugin_Interface(t *testing.T) {
	p := NewServePlugin()
	assert.Equal(t, "serve", p.ID())
	assert.True(t, p.DefaultEnabled())
	assert.Empty(t, p.DependsOn())
	assert.Equal(t, []string{"serve.port"}, p.ConfigSchema())
	assert.Nil(t, p.MCPTools())
}

func TestReadWritePID_RoundTrip(t *testing.T) {
	f := filepath.Join(t.TempDir(), "roundtrip.pid")
	pid := os.Getpid()
	require.NoError(t, os.WriteFile(f, []byte(strconv.Itoa(pid)), 0644))
	got, err := readPID(f)
	require.NoError(t, err)
	assert.Equal(t, pid, got)
}
