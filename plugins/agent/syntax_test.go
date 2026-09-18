package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"
)

// An edit that leaves a file unparseable is worse than one that is refused: the
// model moves on, the build breaks, and the cause is three turns back.

func TestAnEditThatBreaksTheFileIsRefused(t *testing.T) {
	ts, root := aWritingProject(t)
	original := "package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte(original), 0o600))

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "main.go", "old": `println("hello")`, "new": `println("hello"`,
	}))

	require.Contains(t, out, "unparseable")
	after, _ := os.ReadFile(filepath.Join(root, "main.go"))
	require.Equal(t, original, string(after), "and the file is untouched")
}

// The arguments are JSON before they are Go, so a \n meant for a string literal
// arrives as a real line break. It is the commonest way an edit breaks a file.
func TestARealNewlineInAGoStringIsNamedForWhatItIs(t *testing.T) {
	ts, root := aWritingProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc main() {\n\tprintln(\"hello\")\n}\n"), 0o600))

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "main.go",
		"old":  `println("hello")`,
		"new":  "println(\"hello:\n\")",
	}))

	require.Contains(t, out, `must be written \\n`)
}

func TestAWholeFileWriteThatDoesNotParseIsRefused(t *testing.T) {
	ts, root := aWritingProject(t)
	original := "package main\n\nfunc main() {}\n"
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"), []byte(original), 0o600))

	out := ts.Call(context.Background(), call("write_file", map[string]any{
		"path": "main.go", "content": "package main\n\nfunc main() {\n",
	}))

	require.Contains(t, out, "unparseable")
	after, _ := os.ReadFile(filepath.Join(root, "main.go"))
	require.Equal(t, original, string(after))
}

// Only what we can read is checked; a Markdown file is not Go.
func TestAFileThatIsNotGoIsWrittenWhateverIsInIt(t *testing.T) {
	ts, _ := aWritingProject(t)

	out := ts.Call(context.Background(), call("write_file", map[string]any{
		"path": "NOTES.md", "content": "func main() { this is not go\n",
	}))

	require.Contains(t, out, "wrote NOTES.md")
}

func TestAValidEditStillGoesThrough(t *testing.T) {
	ts, root := aWritingProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "main.go"),
		[]byte("package main\n\nfunc main() {\n\tprintln(\"old\")\n}\n"), 0o600))

	out := ts.Call(context.Background(), call("edit_file", map[string]any{
		"path": "main.go", "old": `println("old")`, "new": `println("new")`,
	}))

	require.Contains(t, out, "edited main.go")
}
