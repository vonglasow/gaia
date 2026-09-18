package serve

import (
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
)

// The daemon's state is one file holding one number.

// aHomeWithoutADaemon points the pid and log paths at a directory of the test's own.
func aHomeWithoutADaemon(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	dir := filepath.Join(home, ".config", "gaia")
	require.NoError(t, os.MkdirAll(dir, 0o755))
	return dir
}

func serveCommand(t *testing.T) (*cobra.Command, *bytes.Buffer) {
	t.Helper()
	cmds, err := NewServePlugin().Register(kernel.NewKernel())
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	var out bytes.Buffer
	cmds[0].SetOut(&out)
	return cmds[0], &out
}

func subcommand(t *testing.T, root *cobra.Command, name string) *cobra.Command {
	t.Helper()
	for _, c := range root.Commands() {
		if c.Name() == name {
			return c
		}
	}
	t.Fatalf("no %q subcommand under %q", name, root.Name())
	return nil
}

func TestRegisterExposesServeStopAndStatus(t *testing.T) {
	root, _ := serveCommand(t)

	require.Equal(t, "serve", root.Use)
	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	require.True(t, names["stop"])
	require.True(t, names["status"])
}

func TestStatusOnAMachineWithNoDaemonSaysStopped(t *testing.T) {
	aHomeWithoutADaemon(t)
	root, out := serveCommand(t)
	status := subcommand(t, root, "status")
	status.SetOut(out)

	require.NoError(t, status.RunE(status, nil))
	require.Contains(t, out.String(), "stopped")
}

func TestStatusReportsARunningDaemonAndItsEndpoint(t *testing.T) {
	dir := aHomeWithoutADaemon(t)
	// This process is, by definition, running.
	require.NoError(t, os.WriteFile(filepath.Join(dir, "serve.pid"),
		[]byte(strconv.Itoa(os.Getpid())), 0o600))

	root, out := serveCommand(t)
	status := subcommand(t, root, "status")
	status.SetOut(out)

	require.NoError(t, status.RunE(status, nil))
	require.Contains(t, out.String(), "running")
	require.Contains(t, out.String(), "/mcp", "a running daemon reports where to reach it")
}

// A pid file left behind by a daemon that died is the ordinary case after a reboot.
func TestStatusClearsAPidFileWhoseProcessIsGone(t *testing.T) {
	dir := aHomeWithoutADaemon(t)
	pidFile := filepath.Join(dir, "serve.pid")
	require.NoError(t, os.WriteFile(pidFile, []byte("999999999"), 0o600))

	root, out := serveCommand(t)
	status := subcommand(t, root, "status")
	status.SetOut(out)

	require.NoError(t, status.RunE(status, nil))
	require.Contains(t, out.String(), "stale pid removed")

	_, err := os.Stat(pidFile)
	require.True(t, os.IsNotExist(err), "the stale file is removed, so the next call is clean")
}

func TestStopOnAMachineWithNoDaemonSaysSo(t *testing.T) {
	aHomeWithoutADaemon(t)
	root, out := serveCommand(t)
	stop := subcommand(t, root, "stop")
	stop.SetOut(out)

	require.NoError(t, stop.RunE(stop, nil))
	require.Contains(t, out.String(), "not running")
}

func TestStopClearsAPidFileWhoseProcessIsGone(t *testing.T) {
	dir := aHomeWithoutADaemon(t)
	pidFile := filepath.Join(dir, "serve.pid")
	require.NoError(t, os.WriteFile(pidFile, []byte("999999999"), 0o600))

	root, out := serveCommand(t)
	stop := subcommand(t, root, "stop")
	stop.SetOut(out)

	require.NoError(t, stop.RunE(stop, nil))
	require.Contains(t, out.String(), "stale pid removed")

	_, err := os.Stat(pidFile)
	require.True(t, os.IsNotExist(err))
}

// An unreadable pid file is treated as no daemon at all rather than as a failure.
func TestStopForgivesAPidFileItCannotParse(t *testing.T) {
	dir := aHomeWithoutADaemon(t)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "serve.pid"), []byte("not a pid"), 0o600))

	root, out := serveCommand(t)
	stop := subcommand(t, root, "stop")
	stop.SetOut(out)

	require.NoError(t, stop.RunE(stop, nil))
	require.Contains(t, out.String(), "not running")
}

func TestTheOutputHelpersWriteWhereCobraPoints(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	require.NoError(t, writeStdoutf(cmd, "pid %d\n", 42))
	require.NoError(t, writeStdoutln(cmd, "and a line"))

	require.Equal(t, "pid 42\nand a line\n", out.String())
}

func TestConfigDirFollowsTheHomeInUse(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	require.Equal(t, filepath.Join(home, ".config", "gaia"), configDir())
	require.Equal(t, filepath.Join(home, ".config", "gaia", "serve.pid"), pidPath())
	require.Equal(t, filepath.Join(home, ".config", "gaia", "serve.log"), logPath())
}

// runServe is the fork, not the server.
func TestStartingAgainWhileADaemonRunsForksNothing(t *testing.T) {
	dir := aHomeWithoutADaemon(t)
	t.Setenv("GAIA_DAEMON", "")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "serve.pid"),
		[]byte(strconv.Itoa(os.Getpid())), 0o600))

	root, out := serveCommand(t)

	require.NoError(t, root.RunE(root, nil))
	require.Contains(t, out.String(), "already running")
	require.Contains(t, out.String(), strconv.Itoa(os.Getpid()),
		"the pid is reported so the person can act on the daemon that is in the way")
}

// serve.port used to be declared in the config schema and read by nothing.
func TestTheEndpointFollowsTheConfiguredPort(t *testing.T) {
	dir := aHomeWithoutADaemon(t)
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("serve.port", "9999")
	require.NoError(t, os.WriteFile(filepath.Join(dir, "serve.pid"),
		[]byte(strconv.Itoa(os.Getpid())), 0o600))

	root, out := serveCommand(t)
	status := subcommand(t, root, "status")
	status.SetOut(out)

	require.NoError(t, status.RunE(status, nil))
	require.Contains(t, out.String(), ":9999/mcp")
	require.NotContains(t, out.String(), ":8765/mcp")
}

func TestThePortFallsBackWhenNobodyConfiguredOne(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.Equal(t, "8765", resolvePort())
	require.Equal(t, "http://localhost:8765/mcp", endpoint())

	viper.Set("serve.port", "   ")
	require.Equal(t, "8765", resolvePort(), "a port set to spaces is a port nobody set")
}

// The token is what a client presents.
func TestTheTokenCommandAnswersOnAMachineThatNeverRanTheDaemon(t *testing.T) {
	aHomeWithoutADaemon(t)
	root, out := serveCommand(t)
	token := subcommand(t, root, "token")
	token.SetOut(out)

	require.NoError(t, token.RunE(token, nil))
	require.Len(t, strings.TrimSpace(out.String()), 64)
}

func TestTheTokenCommandIsStableAcrossCalls(t *testing.T) {
	aHomeWithoutADaemon(t)

	root, out := serveCommand(t)
	token := subcommand(t, root, "token")
	token.SetOut(out)
	require.NoError(t, token.RunE(token, nil))
	first := strings.TrimSpace(out.String())

	out.Reset()
	require.NoError(t, token.RunE(token, nil))

	require.Equal(t, first, strings.TrimSpace(out.String()),
		"a token that changed on every call would invalidate every configured client")
}
