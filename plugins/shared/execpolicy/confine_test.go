package execpolicy

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// An allowlist judges the program, and a program is not one thing. `find` reads
// until it is given -delete; `cat` reads whatever it is pointed at. Both were
// allowed outright, and both are how an agent asked to look at a project read
// ~/.ssh or deleted files nobody mentioned.

func confinedTo(t *testing.T) (Policy, string) {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	p := NewDefaultPolicy()
	p.Workspace = root
	return p, root
}

func TestAReadInsideTheWorkspaceStillRuns(t *testing.T) {
	p, root := confinedTo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o600))

	for _, line := range []string{
		"cat main.go", "ls -la", "grep -r TODO .", "go test ./...",
		"git status", "find . -name *.go",
	} {
		t.Run(line, func(t *testing.T) {
			require.Equal(t, Allow, p.Decide(line).Verdict)
		})
	}
}

// The case that was open: cat is allowed, so cat of anything was allowed.
func TestReadingOutsideTheWorkspaceIsConfirmedFirst(t *testing.T) {
	p, _ := confinedTo(t)

	d := p.Decide("cat /etc/passwd")

	require.Equal(t, Confirm, d.Verdict)
	require.Contains(t, d.Reason, "outside")
}

func TestAHomeRelativePathIsConfirmedFirst(t *testing.T) {
	p, _ := confinedTo(t)

	require.Equal(t, Confirm, p.Decide("cat ~/.ssh/id_rsa").Verdict)
}

func TestClimbingOutIsConfirmedFirst(t *testing.T) {
	p, _ := confinedTo(t)

	require.Equal(t, Confirm, p.Decide("cat ../../etc/hosts").Verdict)
}

// -delete makes find the most destructive tool on the allowlist.
func TestAFlagThatTurnsAReaderIntoAWriterIsConfirmedFirst(t *testing.T) {
	p, root := confinedTo(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o600))

	d := p.Decide("find . -name *.go -delete")

	require.Equal(t, Confirm, d.Verdict)
	require.Contains(t, d.Reason, "-delete")
	require.Contains(t, d.Reason, "write")
}

func TestEveryFlagThatRunsSomethingElseIsConfirmedFirst(t *testing.T) {
	p, _ := confinedTo(t)

	for _, line := range []string{
		"find . -exec rm {} ;", "find . -execdir rm {}", "find . -ok rm {}",
		"find . -fprint /tmp/out", "sed -i s/a/b/ main.go",
	} {
		t.Run(line, func(t *testing.T) {
			require.NotEqual(t, Allow, p.Decide(line).Verdict)
		})
	}
}

// Without a workspace nothing is confined, which is what investigate needs: a
// machine is read at /var and /etc or it is not read at all.
func TestWithNoWorkspaceNothingIsConfined(t *testing.T) {
	p := NewDefaultPolicy()

	require.Equal(t, Allow, p.Decide("cat /etc/passwd").Verdict)
}

// A flag is not a path, and neither is a pattern: judging them as paths would
// ask about every command.
func TestArgumentsThatAreNotPathsAreNotJudgedAsPaths(t *testing.T) {
	require.False(t, looksLikeAPath("-la"))
	require.False(t, looksLikeAPath("status"))
	require.False(t, looksLikeAPath("*.go"))
	require.False(t, looksLikeAPath("TODO"))
	require.False(t, looksLikeAPath(""))

	require.True(t, looksLikeAPath("/etc/passwd"))
	require.True(t, looksLikeAPath("~/.ssh/config"))
	require.True(t, looksLikeAPath("../outside"))
	require.True(t, looksLikeAPath("plugins/ask/plugin.go"))
}

func TestAWritingFlagIsFoundWhereverItSits(t *testing.T) {
	require.Equal(t, "-delete", writingFlag([]string{"find", ".", "-name", "x", "-delete"}))
	require.Equal(t, "-exec", writingFlag([]string{"find", "-exec", "rm"}))
	require.Equal(t, "-fprintf", writingFlag([]string{"find", "-fprintf=/tmp/x"}))
	require.Empty(t, writingFlag([]string{"find", ".", "-name", "*.go"}))
	require.Empty(t, writingFlag([]string{"git", "status"}))
}

// -i is sed's in-place, and grep's ignore-case. Judging it by name alone would
// ask about `grep -i`, which is the most ordinary command there is.
func TestTheSameFlagIsJudgedByTheProgramItBelongsTo(t *testing.T) {
	require.Equal(t, "-i", writingFlag([]string{"sed", "-i", "s/a/b/", "f"}))
	require.Empty(t, writingFlag([]string{"grep", "-i", "todo"}))
}

// Confirmation and not refusal: with a terminal a person can say yes knowing
// what they are agreeing to, and with none the guard already fails closed.
func TestConfinementAsksRatherThanRefuses(t *testing.T) {
	p, _ := confinedTo(t)

	require.Equal(t, Confirm, p.Decide("cat /etc/passwd").Verdict)
	require.Equal(t, Refuse, p.Decide("sudo cat /etc/passwd").Verdict)
}

// A refusal still outranks everything: --yes is a person deciding in advance,
// and confinement is not what it decides about.
func TestWaivingConfirmationReachesConfinementToo(t *testing.T) {
	p, _ := confinedTo(t)
	p.AllowAll = true

	require.Equal(t, Allow, p.Decide("cat /etc/passwd").Verdict)
	require.Equal(t, Refuse, p.Decide("sudo ls").Verdict)
}

func TestTheWorkspaceItselfIsInside(t *testing.T) {
	p, root := confinedTo(t)

	require.Equal(t, Allow, p.Decide("ls "+root).Verdict)
}

func TestASiblingWithALongerNameIsOutside(t *testing.T) {
	parent, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.MkdirAll(filepath.Join(parent, "work"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(parent, "work-elsewhere"), 0o755))

	p := NewDefaultPolicy()
	p.Workspace = filepath.Join(parent, "work")

	require.Equal(t, Confirm, p.Decide("ls "+filepath.Join(parent, "work-elsewhere")).Verdict)
}

// A symlink inside the workspace pointing out of it passes a prefix check on
// the text and lands elsewhere on disk.
func TestASymlinkOutOfTheWorkspaceIsCaught(t *testing.T) {
	p, root := confinedTo(t)
	outside, err := filepath.EvalSymlinks(t.TempDir())
	require.NoError(t, err)
	require.NoError(t, os.Symlink(outside, filepath.Join(root, "escape")))

	require.Equal(t, Confirm, p.Decide("ls escape").Verdict)
}

func TestConfinementSurvivesExtraEntries(t *testing.T) {
	p, _ := confinedTo(t)
	extended := p.WithExtra([]string{"npm"}, nil)

	require.Equal(t, Confirm, extended.Decide("cat /etc/passwd").Verdict,
		"adding an allowance must not drop the workspace")
}

// A flag written as -name=value is the same flag, and the value is not part of it.
func TestAFlagIsRecognisedWithOrWithoutItsValue(t *testing.T) {
	require.Equal(t, "-fprintf", writingFlag([]string{"find", "-fprintf=/tmp/x"}))
	require.Equal(t, "-fprintf", writingFlag([]string{"find", "-fprintf", "/tmp/x"}))
	require.Empty(t, writingFlag([]string{"find", "=weird"}))
}

// A workspace that is not there confines nothing rather than refusing everything:
// the caller has a worse problem, and it is not this function's to report.
func TestAWorkspaceThatIsNotThereFallsBackToTheTextOfThePath(t *testing.T) {
	p := NewDefaultPolicy()
	p.Workspace = filepath.Join(t.TempDir(), "never-created")

	require.Equal(t, Confirm, p.Decide("cat /etc/passwd").Verdict)
}
