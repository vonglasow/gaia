package version

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"

	"gaia/kernel"
)

// What `gaia version` prints is the only way to tell which build is installed.

// stampsAre sets the three linker-written variables for one test and puts them back.
func stampsAre(t *testing.T, version, commit, date string) {
	t.Helper()
	previousVersion, previousCommit, previousDate := Version, Commit, Date
	t.Cleanup(func() { Version, Commit, Date = previousVersion, previousCommit, previousDate })
	Version, Commit, Date = version, commit, date
}

func TestInfoReportsTheStampsItWasBuiltWith(t *testing.T) {
	stampsAre(t, "v2.16.0", "0123456789abcdef0123", "2026-09-16T10:00:00Z")

	info := Info()

	require.Contains(t, info, "Gaia v2.16.0")
	require.Contains(t, info, "2026-09-16T10:00:00Z")
}

// A long commit is shortened to something a person can read off a terminal and still.
func TestInfoShortensALongCommit(t *testing.T) {
	stampsAre(t, "v1.0.0", "0123456789abcdef0123456789abcdef01234567", "2026-09-16T10:00:00Z")

	require.Contains(t, Info(), "commit 0123456789ab")
	require.NotContains(t, Info(), "0123456789abcdef")
}

func TestInfoLeavesAShortCommitAlone(t *testing.T) {
	stampsAre(t, "v1.0.0", "abc1234", "2026-09-16T10:00:00Z")

	require.Contains(t, Info(), "commit abc1234")
}

// This is the case a released gaia is actually in today.
func TestInfoStillAnswersWhenNothingWasStamped(t *testing.T) {
	stampsAre(t, "", "", "")

	info := Info()

	require.Contains(t, info, "Gaia ")
	require.Contains(t, info, "commit ")
	require.Contains(t, info, "built at ")
	require.NotContains(t, info, "commit \n")
}

func TestInfoNormalisesADateItCanParse(t *testing.T) {
	stampsAre(t, "v1.0.0", "abc1234", "2026-09-16T12:00:00+02:00")

	require.Contains(t, Info(), "2026-09-16T10:00:00Z", "the date is reported in UTC")
}

func TestInfoKeepsADateItCannotParse(t *testing.T) {
	stampsAre(t, "v1.0.0", "abc1234", "last tuesday")

	require.Contains(t, Info(), "last tuesday")
}

func TestFirstNonEmptySkipsBlanks(t *testing.T) {
	require.Equal(t, "second", firstNonEmpty("", "   ", "second", "third"))
	require.Equal(t, "", firstNonEmpty("", "  "))
}

func TestReadBuildSettingAnswersEmptyForAKeyNobodyWrote(t *testing.T) {
	require.Equal(t, "", readBuildSetting("no.such.setting"))
}

func TestThePluginDeclaresItselfAndRegistersOneCommand(t *testing.T) {
	p := NewVersionPlugin()

	require.Equal(t, "version", p.ID())
	require.True(t, p.DefaultEnabled())
	require.Nil(t, p.DependsOn())
	require.Nil(t, p.ConfigSchema())
	require.Nil(t, p.MCPTools())

	cmds, err := p.Register(kernel.NewKernel())
	require.NoError(t, err)
	require.Len(t, cmds, 1)
	require.Equal(t, "version", cmds[0].Use)
}

func TestTheCommandWritesToTheWriterItWasGiven(t *testing.T) {
	stampsAre(t, "v9.9.9", "abc1234", "2026-09-16T10:00:00Z")

	cmds, err := NewVersionPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)

	var out bytes.Buffer
	cmds[0].SetOut(&out)
	require.NoError(t, cmds[0].RunE(cmds[0], nil))

	require.Contains(t, out.String(), "v9.9.9",
		"the command prints where cobra points it, which is what lets a test read it at all")
}
