package cache

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
	"gaia/plugins/shared"
)

// The commands are how a person finds out what the cache is holding and gets rid of it.

func cacheCommands(t *testing.T) (*cobra.Command, *bytes.Buffer, *bytes.Buffer) {
	t.Helper()
	cmds, err := NewCachePlugin().Register(kernel.NewKernel())
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	var out, errOut bytes.Buffer
	cmds[0].SetOut(&out)
	cmds[0].SetErr(&errOut)
	return cmds[0], &out, &errOut
}

func sub(t *testing.T, root *cobra.Command, name string, out, errOut *bytes.Buffer) *cobra.Command {
	t.Helper()
	for _, c := range root.Commands() {
		if c.Name() == name {
			c.SetOut(out)
			c.SetErr(errOut)
			return c
		}
	}
	t.Fatalf("no %q subcommand", name)
	return nil
}

func TestThePluginDeclaresItselfToTheKernel(t *testing.T) {
	p := NewCachePlugin()

	require.Equal(t, "cache", p.ID())
	require.True(t, p.DefaultEnabled())
	require.Nil(t, p.DependsOn())
	require.Nil(t, p.MCPTools())
	require.NotEmpty(t, p.ConfigSchema())
}

func TestEverySubcommandIsRegistered(t *testing.T) {
	root, _, _ := cacheCommands(t)

	names := map[string]bool{}
	for _, c := range root.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"list", "show", "stats", "clear", "delete"} {
		require.Truef(t, names[want], "subcommand %q is missing", want)
	}
}

func TestListingAnEmptyCacheSaysSoRatherThanPrintingNothing(t *testing.T) {
	useTempCache(t)
	root, out, errOut := cacheCommands(t)
	list := sub(t, root, "list", out, errOut)

	require.NoError(t, list.RunE(list, nil))
	require.NotEmpty(t, out.String(), "an empty box still tells the person the cache was read")
}

func TestListingReportsEachEntry(t *testing.T) {
	useTempCache(t)
	require.NoError(t, Set(Entry{Key: "abc", Label: "a question", PluginID: "ask", Model: "llama3.1"}))

	root, out, errOut := cacheCommands(t)
	list := sub(t, root, "list", out, errOut)

	require.NoError(t, list.RunE(list, nil))
	require.Contains(t, out.String(), "abc")
	require.Contains(t, out.String(), "ask")
	require.Contains(t, out.String(), "a question")
}

func TestShowingAnEntryPrintsTheAnswerItHolds(t *testing.T) {
	useTempCache(t)
	require.NoError(t, Set(Entry{Key: "abc", Label: "a question", Response: "the cached answer"}))

	root, out, errOut := cacheCommands(t)
	show := sub(t, root, "show", out, errOut)

	require.NoError(t, show.RunE(show, []string{"abc"}))
	require.Contains(t, out.String(), "the cached answer")
}

// A miss is a failure the shell can act on, and the reason goes to standard error.
func TestShowingAnEntryThatIsNotThereFailsAndSaysWhy(t *testing.T) {
	useTempCache(t)
	root, out, errOut := cacheCommands(t)
	show := sub(t, root, "show", out, errOut)

	err := show.RunE(show, []string{"never-written"})

	require.ErrorIs(t, err, shared.ErrReported)
	require.NotEmpty(t, errOut.String())
	require.Empty(t, out.String(), "a miss is a reason, not a result")
}

func TestStatsCountsWhatIsStored(t *testing.T) {
	useTempCache(t)
	require.NoError(t, Set(Entry{Key: "one"}))
	require.NoError(t, Set(Entry{Key: "two"}))

	root, out, errOut := cacheCommands(t)
	stats := sub(t, root, "stats", out, errOut)

	require.NoError(t, stats.RunE(stats, nil))
	require.Contains(t, out.String(), "2")
}

// Clearing is the one command that destroys something.
func TestClearingRemovesEveryEntryAndSaysHowMany(t *testing.T) {
	useTempCache(t)
	require.NoError(t, Set(Entry{Key: "one"}))
	require.NoError(t, Set(Entry{Key: "two"}))

	root, out, errOut := cacheCommands(t)
	clearCmd := sub(t, root, "clear", out, errOut)

	require.NoError(t, clearCmd.RunE(clearCmd, nil))
	require.Contains(t, out.String(), "2")

	entries, err := List()
	require.NoError(t, err)
	require.Empty(t, entries)
}

func TestDeletingRemovesOneEntryAndLeavesTheRest(t *testing.T) {
	useTempCache(t)
	require.NoError(t, Set(Entry{Key: "one"}))
	require.NoError(t, Set(Entry{Key: "two"}))

	root, out, errOut := cacheCommands(t)
	del := sub(t, root, "delete", out, errOut)

	require.NoError(t, del.RunE(del, []string{"one"}))

	entries, err := List()
	require.NoError(t, err)
	require.Len(t, entries, 1)
	require.Equal(t, "two", entries[0].Key)
}

func TestShowAndDeleteTakeExactlyOneKey(t *testing.T) {
	root, out, errOut := cacheCommands(t)

	show := sub(t, root, "show", out, errOut)
	require.Error(t, show.Args(nil, []string{}))
	require.NoError(t, show.Args(nil, []string{"abc"}))

	del := sub(t, root, "delete", out, errOut)
	require.Error(t, del.Args(nil, []string{"abc", "def"}))
}

// Without cache.dir configured the store lands under the home directory.
func TestTheCacheDirectoryDefaultsToTheHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	viper.Reset()
	t.Cleanup(viper.Reset)

	dir, err := getCacheDir()

	require.NoError(t, err)
	require.Equal(t, filepath.Join(home, ".config", "gaia", "cache"), dir)
}

func TestAConfiguredCacheDirectoryWins(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("cache.dir", "  /somewhere/else  ")

	dir, err := getCacheDir()

	require.NoError(t, err)
	require.Equal(t, "/somewhere/else", dir, "the configured path is trimmed, not taken with its spaces")
}

func TestAnEntryWrittenIsOnlyReadableByItsOwner(t *testing.T) {
	dir := useTempCache(t)
	require.NoError(t, Set(Entry{Key: "abc", Response: "a question and its answer"}))

	info, err := os.Stat(filepath.Join(dir, "abc.json"))

	require.NoError(t, err)
	require.Equal(t, os.FileMode(0o600), info.Mode().Perm(),
		"cached prompts and answers are as private as the questions that produced them")
}
