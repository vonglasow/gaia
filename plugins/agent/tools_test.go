package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"gaia/plugins/ask"
	"gaia/plugins/shared/execpolicy"
)

// What the tools do is half the design.

func aProject(t *testing.T) (*Toolset, string) {
	t.Helper()
	ws, root := aWorkspace(t)
	return NewToolset(ws, DefaultPermissions()), root
}

func aWritingProject(t *testing.T) (*Toolset, string) {
	t.Helper()
	ws, root := aWorkspace(t)
	perms := DefaultPermissions()
	perms.AllowWrites = true
	return NewToolset(ws, perms), root
}

func call(name string, args map[string]any) ask.ToolCall {
	return ask.ToolCall{Name: name, Arguments: args}
}

func TestReadingAFileNumbersItsLines(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc main() {}\n"), 0o600))

	out := ts.Call(context.Background(), call("read_file", map[string]any{"path": "main.go"}))

	require.Contains(t, out, "1\tpackage main")
	require.Contains(t, out, "3\tfunc main() {}",
		"the next thing a model does with a file is talk about a line in it")
}

func TestReadingARangeReadsOnlyThatRange(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "long.go"),
		[]byte("one\ntwo\nthree\nfour\nfive\n"), 0o600))

	out := ts.Call(context.Background(), call("read_file",
		map[string]any{"path": "long.go", "start": 2, "end": 3}))

	require.Contains(t, out, "two")
	require.Contains(t, out, "three")
	require.NotContains(t, out, "one")
	require.NotContains(t, out, "five")
}

// One vendored file would otherwise fill the context window.
func TestAVeryLargeFileIsTruncatedAndSaysSo(t *testing.T) {
	ws, root := aWorkspace(t)
	perms := DefaultPermissions()
	perms.MaxFileBytes = 200
	ts := NewToolset(ws, perms)
	require.NoError(t, os.WriteFile(filepath.Join(root, "big.go"),
		[]byte(strings.Repeat("a line of code\n", 500)), 0o600))

	out := ts.Call(context.Background(), call("read_file", map[string]any{"path": "big.go"}))

	require.Less(t, len(out), 600)
	require.Contains(t, out, "truncated")
	require.Contains(t, out, "start and end", "and says how to read the rest")
}

func TestReadingOutsideTheProjectIsRefused(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("read_file", map[string]any{"path": "../../etc/passwd"}))

	require.Contains(t, out, "error:")
	require.Contains(t, out, "outside the workspace")
}

// A failing tool answers with its reason rather than stopping the run.
func TestAFileThatIsNotThereAnswersWithTheReason(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("read_file", map[string]any{"path": "nope.go"}))

	require.Contains(t, out, "error:")
	require.Contains(t, out, "no such file")
}

func TestListingHidesWhatAModelShouldNotWadeThrough(t *testing.T) {
	ts, root := aProject(t)
	for _, dir := range []string{".git", "node_modules", "vendor", "plugins"} {
		require.NoError(t, os.MkdirAll(filepath.Join(root, dir), 0o755))
	}
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o600))

	out := ts.Call(context.Background(), call("list_files", nil))

	require.Contains(t, out, "main.go")
	require.Contains(t, out, "plugins/")
	require.NotContains(t, out, ".git",
		"a model handed the contents of .git spends its steps reading object files")
	require.NotContains(t, out, "node_modules")
}

func TestSearchingFindsTheFileAndTheLine(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "plugins"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(root, "plugins", "a.go"),
		[]byte("package a\n\nfunc Target() {}\n"), 0o600))

	out := ts.Call(context.Background(), call("search_text", map[string]any{"pattern": "func Target"}))

	require.Contains(t, out, filepath.Join("plugins", "a.go")+":3")
	require.Contains(t, out, "func Target")
}

// Nothing found is an answer a model has to be able to act on.
func TestSearchingForWhatIsNotThereSaysSo(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("search_text", map[string]any{"pattern": "NoSuchSymbol"}))

	require.Contains(t, out, "no file")
	require.Contains(t, out, "NoSuchSymbol")
}

func TestSearchingForNothingIsRefused(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("search_text", map[string]any{"pattern": "   "}))

	require.Contains(t, out, "every line in the project")
}

func TestSearchingSkipsBinaryFiles(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "binary.bin"),
		append([]byte("needle"), 0x00, 0x01, 0x02), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "text.go"), []byte("needle\n"), 0o600))

	out := ts.Call(context.Background(), call("search_text", map[string]any{"pattern": "needle"}))

	require.Contains(t, out, "text.go")
	require.NotContains(t, out, "binary.bin")
}

// --- writing --------------------------------------------------------------

// A model is never offered a tool it would then be refused.
func TestAReadOnlyAgentIsNotOfferedWriteFile(t *testing.T) {
	ts, _ := aProject(t)

	require.NotContains(t, ts.Names(), "write_file")

	out := ts.Call(context.Background(), call("write_file",
		map[string]any{"path": "x.go", "content": "x"}))
	require.Contains(t, out, "no tool called")
}

func TestAWritingAgentWritesAndReportsWhatItWrote(t *testing.T) {
	ts, root := aWritingProject(t)

	out := ts.Call(context.Background(), call("write_file",
		map[string]any{"path": "new.go", "content": "package main\n"}))

	require.Contains(t, out, "wrote new.go")
	data, err := os.ReadFile(filepath.Join(root, "new.go"))
	require.NoError(t, err)
	require.Equal(t, "package main\n", string(data))
}

func TestWritingCreatesTheDirectoriesItNeeds(t *testing.T) {
	ts, root := aWritingProject(t)

	ts.Call(context.Background(), call("write_file",
		map[string]any{"path": "plugins/new/plugin.go", "content": "package new\n"}))

	_, err := os.Stat(filepath.Join(root, "plugins", "new", "plugin.go"))
	require.NoError(t, err)
}

func TestWritingOutsideTheProjectIsRefused(t *testing.T) {
	ts, _ := aWritingProject(t)

	out := ts.Call(context.Background(), call("write_file",
		map[string]any{"path": "../escaped.go", "content": "x"}))

	require.Contains(t, out, "outside the workspace")
}

// An agent that touched forty files when asked to fix one is the thing to notice.
func TestEveryFileWrittenIsRecorded(t *testing.T) {
	ts, _ := aWritingProject(t)

	ts.Call(context.Background(), call("write_file", map[string]any{"path": "a.txt", "content": "a"}))
	ts.Call(context.Background(), call("write_file", map[string]any{"path": "b.txt", "content": "b"}))
	ts.Call(context.Background(), call("write_file", map[string]any{"path": "a.txt", "content": "a2"}))

	require.Equal(t, []string{"a.txt", "b.txt"}, ts.FilesWritten(),
		"written twice is one file, and the list is sorted so two runs are comparable")
}

func TestNothingWrittenIsAnEmptyList(t *testing.T) {
	ts, _ := aProject(t)
	require.Empty(t, ts.FilesWritten())
}

// --- running --------------------------------------------------------------

func TestARunningToolReportsBothStreamsAndTheExitCode(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "echo hello"}))

	require.Contains(t, out, "hello")
	require.Contains(t, out, "exit code: 0",
		"a test run that passed and one that failed can look alike; the code is what tells them apart")
}

func TestAFailingCommandStillReportsItsCode(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "ls no-such-place"}))

	require.Contains(t, out, "exit code:")
	require.NotContains(t, out, "exit code: 0")
}

func TestARefusedCommandDoesNotRun(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "sudo rm -rf /"}))

	require.Contains(t, out, "refused")
}

// With nobody to confirm, a command off the allowlist does not run.
func TestACommandNeedingConfirmationDoesNotRunWithNobodyToAsk(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "rm -rf build"}))

	require.Contains(t, out, "nobody to ask")
}

func TestAConfirmedCommandRuns(t *testing.T) {
	ws, _ := aWorkspace(t)
	perms := DefaultPermissions()
	perms.ConfirmRun = func(string) (bool, error) { return true, nil }
	ts := NewToolset(ws, perms)

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "true"}))

	require.Contains(t, out, "exit code: 0")
}

func TestADeclinedCommandDoesNotRun(t *testing.T) {
	ws, _ := aWorkspace(t)
	perms := DefaultPermissions()
	perms.ConfirmRun = func(string) (bool, error) { return false, nil }
	ts := NewToolset(ws, perms)

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "rm -rf build"}))

	require.Contains(t, out, "declined")
}

// The project's checks run against the project.
func TestCommandsRunInsideTheProject(t *testing.T) {
	ts, root := aProject(t)

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "pwd"}))

	require.Contains(t, out, filepath.Base(root))
}

func TestShellSyntaxIsRefusedFromAToolToo(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("run_command",
		map[string]any{"command": "echo hi > /tmp/escaped"}))

	require.Contains(t, out, "shell syntax")
}

// --- what the model is told about -----------------------------------------

func TestEveryToolIsDescribedWithASchema(t *testing.T) {
	ts, _ := aWritingProject(t)

	specs := ts.Specs()
	require.NotEmpty(t, specs)

	for _, spec := range specs {
		require.NotEmptyf(t, spec.Description, "tool %s has no description", spec.Name)
		require.Equalf(t, "object", spec.Parameters["type"], "tool %s", spec.Name)
	}
}

// Two runs of the same task should differ because the model differed.
func TestTheToolsAreOfferedInAStableOrder(t *testing.T) {
	ts, _ := aWritingProject(t)

	first := ts.Names()
	for range 5 {
		require.Equal(t, first, ts.Names())
	}
}

func TestAToolNobodyRegisteredSaysWhatDoesExist(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("delete_everything", nil))

	require.Contains(t, out, "no tool called")
	require.Contains(t, out, "read_file", "the answer says what it could have called instead")
}

// "1\t" reads as a tool that half worked.
func TestAnEmptyFileSaysThatItIsEmpty(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "empty.go"), nil, 0o600))

	out := ts.Call(context.Background(), call("read_file", map[string]any{"path": "empty.go"}))

	require.Equal(t, "empty.go is empty", out)
}

// And a tool that genuinely returns nothing says so too.
func TestAnEmptyResultSaysThatItIsEmpty(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.MkdirAll(filepath.Join(root, "nothing"), 0o755))

	out := ts.Call(context.Background(), call("list_files", map[string]any{"path": "nothing"}))

	require.Equal(t, "(no output)", out)
}

func TestTheDefaultAgentReadsAndDoesNotWrite(t *testing.T) {
	perms := DefaultPermissions()

	require.False(t, perms.AllowWrites, "the useful thing that needs nobody's decision is a read-only agent")
	require.Positive(t, perms.MaxFileBytes)
	require.Positive(t, perms.CommandTimeout)
	require.Equal(t, execpolicy.Allow, perms.Policy.Decide("git status").Verdict)
}

func TestACommandThatWillNotEndIsStopped(t *testing.T) {
	ws, _ := aWorkspace(t)
	perms := DefaultPermissions()
	perms.CommandTimeout = 200 * time.Millisecond
	perms.ConfirmRun = func(string) (bool, error) { return true, nil }
	ts := NewToolset(ws, perms)

	start := time.Now()
	ts.Call(context.Background(), call("run_command", map[string]any{"command": "sleep 30"}))

	require.Less(t, time.Since(start), 5*time.Second)
}

// Measured, not supposed: a search of gaia for "execpolicy" returned a hundred lines.
func TestSearchingIgnoresWhatGitIgnores(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("needle\n"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, "coverage.out"),
		[]byte(strings.Repeat("needle in a generated profile\n", 200)), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(root, ".gitignore"), []byte("coverage.out\n"), 0o600))

	initGit(t, root)

	out := ts.Call(context.Background(), call("search_text", map[string]any{"pattern": "needle"}))

	require.Contains(t, out, "main.go")
	require.NotContains(t, out, "coverage.out",
		"a generated file fills the result and buries the code the model was looking for")
}

// Outside a repository there is nothing to ask git, so the walk is the answer.
func TestSearchingWorksOutsideARepository(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("needle\n"), 0o600))

	out := ts.Call(context.Background(), call("search_text", map[string]any{"pattern": "needle"}))

	require.Contains(t, out, "main.go")
}

func initGit(t *testing.T, root string) {
	t.Helper()
	for _, argv := range [][]string{
		{"git", "init", "-q"},
		{"git", "add", "main.go", ".gitignore"},
	} {
		_, err := execpolicy.RunIn(context.Background(), root, argv, 30*time.Second)
		require.NoError(t, err)
	}
}

// An allowlist judges the program, not what it is pointed at: `cat` was allowed
// and so was `cat ~/.ssh/id_rsa`, from an agent that could not write a file.
func TestReadingOutsideTheProjectNeedsConfirmation(t *testing.T) {
	ts, _ := aProject(t)
	outside := filepath.Join(t.TempDir(), "secret")
	require.NoError(t, os.WriteFile(outside, []byte("SECRET"), 0o600))

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "cat " + outside}))

	require.NotContains(t, out, "SECRET")
	require.Contains(t, out, "nobody to ask")
}

// find reads until it is given -delete, and then it is the most destructive
// tool on the allowlist. A read-only agent deleted files with it.
func TestAFlagThatDeletesNeedsConfirmation(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("x"), 0o600))

	out := ts.Call(context.Background(), call("run_command",
		map[string]any{"command": "find . -name *.go -delete"}))

	require.Contains(t, out, "nobody to ask")
	_, err := os.Stat(filepath.Join(root, "main.go"))
	require.NoError(t, err, "the file is still there")
}

func TestReadingInsideTheProjectStillRunsWithoutAsking(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main"), 0o600))

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "cat main.go"}))

	require.Contains(t, out, "package main")
}

// investigate is deliberately not confined: a machine is read at /var and /etc
// or it is not read at all.
func TestACommandToolsetReadsTheWholeMachine(t *testing.T) {
	ws, _ := aWorkspace(t)
	ts := NewCommandToolset(ws, DefaultPermissions())
	outside := filepath.Join(t.TempDir(), "elsewhere")
	require.NoError(t, os.WriteFile(outside, []byte("READABLE"), 0o600))

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "cat " + outside}))

	require.Contains(t, out, "READABLE")
}

// Mutation testing found these: lines the suite ran and never asserted on.

// A range asked for backwards, or past the end, is a model guessing — it gets
// the file clamped rather than an error to reason about.
func TestARangeOutsideTheFileIsBroughtBackInside(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "short.go"),
		[]byte("one\ntwo\nthree\n"), 0o600))

	for _, given := range []map[string]any{
		{"path": "short.go", "start": 0},
		{"path": "short.go", "start": "not a number"},
		{"path": "short.go", "end": 900},
		{"path": "short.go", "start": 3, "end": 1},
	} {
		out := ts.Call(context.Background(), call("read_file", given))
		require.NotContains(t, out, "Error", "%v", given)
	}
}

func TestARangeStartingPastTheEndReadsTheLastLine(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "short.go"),
		[]byte("one\ntwo\nthree"), 0o600))

	out := ts.Call(context.Background(), call("read_file",
		map[string]any{"path": "short.go", "start": 900}))

	require.Contains(t, out, "three")
	require.NotContains(t, out, "one")
}

// A file larger than the budget is cut, and says it was cut: a model given
// half a file without being told would answer about the half it saw.
func TestAFileTooLargeIsCutAndSaysSo(t *testing.T) {
	ws, root := aWorkspace(t)
	perms := DefaultPermissions()
	perms.MaxFileBytes = 40
	ts := NewToolset(ws, perms)
	require.NoError(t, os.WriteFile(filepath.Join(root, "big.go"),
		[]byte(strings.Repeat("a line of text\n", 50)), 0o600))

	out := ts.Call(context.Background(), call("read_file", map[string]any{"path": "big.go"}))

	require.Contains(t, out, "truncated at 40 bytes")
	require.Less(t, len(out), 400)
}

// The same cap on a search: a pattern matching everything must not return the
// repository.
func TestASearchThatMatchesEverythingStopsAndSaysWhere(t *testing.T) {
	ts, root := aProject(t)
	for i := range 5 {
		require.NoError(t, os.WriteFile(filepath.Join(root, fmt.Sprintf("f%d.go", i)),
			[]byte(strings.Repeat("match\n", 40)), 0o600))
	}

	out := ts.Call(context.Background(), call("search_text", map[string]any{"pattern": "match"}))

	require.Contains(t, out, "stopped at")
	require.Contains(t, out, "search something narrower")
}

// What a command wrote to stderr is labelled, so a model does not read a
// compiler's complaint as its output.
func TestACommandsErrorOutputIsKeptAndLabelled(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "ls no-such-place"}))

	require.Contains(t, out, "stderr:")
	require.Contains(t, out, "no-such-place")
}

// Nothing on stdout means nothing printed, not a blank line before the code.
func TestACommandThatPrintsNothingAddsNothing(t *testing.T) {
	ws, _ := aWorkspace(t)
	perms := DefaultPermissions()
	perms.ConfirmRun = func(string) (bool, error) { return true, nil }
	ts := NewToolset(ws, perms)

	out := ts.Call(context.Background(), call("run_command", map[string]any{"command": "true"}))

	require.Equal(t, "exit code: 0", out)
}

// --- editing ---------------------------------------------------------------

// A model asked to change five lines of five hundred cannot reproduce the other
// four hundred and ninety-five, and writes the five. edit_file is how it says
// what it means without holding the whole file.

func TestEditingReplacesJustThatText(t *testing.T) {
	ts, root := aWritingProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"), 0o600))

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "main.go", "old": `println("old")`, "new": `println("new")`,
	}))

	require.Contains(t, out, "edited main.go")
	after, err := os.ReadFile(filepath.Join(root, "main.go"))
	require.NoError(t, err)
	require.Contains(t, string(after), `println("new")`)
	require.Contains(t, string(after), "package main", "the rest of the file is untouched")
}

func TestEditingTextThatIsNotThereSaysSoAndChangesNothing(t *testing.T) {
	ts, root := aWritingProject(t)
	original := "package main\n\nfunc main() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte(original), 0o600))

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "main.go", "old": "func other()", "new": "func another()",
	}))

	require.Contains(t, out, "not in main.go")
	after, _ := os.ReadFile(filepath.Join(root, "main.go"))
	require.Equal(t, original, string(after))
}

// Two matches means the model did not say which, and guessing would corrupt the
// other one silently.
func TestEditingAmbiguousTextIsRefused(t *testing.T) {
	ts, root := aWritingProject(t)
	original := "a := 1\nb := 1\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte(original), 0o600))

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "main.go", "old": ":= 1", "new": ":= 2",
	}))

	require.Contains(t, out, "more than once")
	after, _ := os.ReadFile(filepath.Join(root, "main.go"))
	require.Equal(t, original, string(after))
}

// Without --write the tool is not offered at all, which is a plainer refusal
// than offering it and saying no.
func TestEditingIsNotEvenOfferedWithoutWritePermission(t *testing.T) {
	ts, _ := aProject(t)

	require.NotContains(t, ts.Names(), "edit_file")
	require.NotContains(t, ts.Names(), "write_file")
}

// This is the failure that cost a 573-line file: asked to change part of it, a
// 14B model called write_file with 295 bytes and deleted the rest.
func TestAWholeFileWriteThatWouldDeleteMostOfItIsRefused(t *testing.T) {
	ts, root := aWritingProject(t)
	original := strings.Repeat("a line of a long file\n", 40)
	require.NoError(t, os.WriteFile(filepath.Join(root, "long.txt"), []byte(original), 0o600))

	out := ts.Call(context.Background(), call("write_file", map[string]any{
		"path": "long.txt", "content": "a line of a long file\n",
	}))

	require.Contains(t, out, "refusing to write")
	require.Contains(t, out, "edit_file", "and says what to do instead")
	after, _ := os.ReadFile(filepath.Join(root, "long.txt"))
	require.Equal(t, original, string(after), "the file is exactly as it was")
}

func TestAWholeFileWriteThatKeepsMostOfItIsAllowed(t *testing.T) {
	ts, root := aWritingProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "long.txt"),
		[]byte(strings.Repeat("a line of a long file\n", 40)), 0o600))

	out := ts.Call(context.Background(), call("write_file", map[string]any{
		"path": "long.txt", "content": strings.Repeat("a rewritten line\n", 40),
	}))

	require.Contains(t, out, "wrote long.txt")
}

// A new file has nothing to lose, and a short one has no ratio worth reading.
func TestANewFileIsWrittenWhateverItsSize(t *testing.T) {
	ts, _ := aWritingProject(t)

	out := ts.Call(context.Background(), call("write_file", map[string]any{
		"path": "notes.md", "content": "one line\n",
	}))

	require.Contains(t, out, "wrote notes.md")
}

func TestAShortFileIsReplacedWithoutComplaint(t *testing.T) {
	ts, root := aWritingProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "small.go"), []byte("package main\n"), 0o600))

	out := ts.Call(context.Background(), call("write_file", map[string]any{
		"path": "small.go", "content": "package x\n",
	}))

	require.Contains(t, out, "wrote small.go")
	require.NotContains(t, out, "unparseable")
}

// read_file is the only copy of the file a model has, and it numbers its lines.
// Quoting straight back from it must work, or edit_file is unusable.
func TestEditingAcceptsTextQuotedBackWithItsLineNumbers(t *testing.T) {
	ts, root := aWritingProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"), 0o600))

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "main.go",
		"old":  "3\tfunc main() {\n4\t\tprintln(\"old\")",
		"new":  "3\tfunc main() {\n4\t\tprintln(\"new\")",
	}))

	require.Contains(t, out, "edited main.go")
	after, _ := os.ReadFile(filepath.Join(root, "main.go"))
	require.Contains(t, string(after), `println("new")`)
	require.NotContains(t, string(after), "3\t", "the numbers are not part of the file")
}

// A model that guessed wrong needs to see what is there, not be told to guess
// again. It has already read the file once.
func TestAFailedEditShowsWhatIsActuallyInTheFile(t *testing.T) {
	ts, root := aWritingProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\n// BuildThing makes a thing from nothing.\nfunc BuildThing() {}\n"), 0o600))

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "main.go",
		"old":  "// BuildThing creates a thing for the given input.",
		"new":  "// BuildThing makes one.",
	}))

	require.Contains(t, out, "The closest thing in the file is")
	require.Contains(t, out, "// BuildThing makes a thing from nothing.")
}

func TestAFailedEditWithNothingRecognisableJustSaysToReadAgain(t *testing.T) {
	ts, root := aWritingProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte("package main\n"), 0o600))

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "main.go", "old": "zzz qqq xxx", "new": "something",
	}))

	require.Contains(t, out, "copy the lines exactly")
}

// A model that reaches for the wrong tool repeats the mistake until something
// tells it which one it wanted. Five identical failures ended one run.
func TestCallingTheWrongToolNamesTheRightOne(t *testing.T) {
	ts, _ := aWritingProject(t)

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"command": "go test ./...",
	}))

	require.Contains(t, out, "belong to run_command")
}

func TestAnOrdinaryMistakeIsNotBlamedOnAnotherTool(t *testing.T) {
	ts, _ := aWritingProject(t)

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "nope.go", "old": "a", "new": "b",
	}))

	require.NotContains(t, out, "belong to")
}
