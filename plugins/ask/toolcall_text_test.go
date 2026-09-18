package ask

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// A smaller model sometimes writes the call it means to make instead of making
// it, and the run stops one step from the answer. This is that step.

var theTools = []string{"read_file", "edit_file", "write_file", "run_command"}

func TestACallWrittenAsTextIsRecovered(t *testing.T) {
	text := `Now, let's modify inject.go:

{"name": "edit_file", "arguments": {"path": "plugins/mempalace/inject.go", "old": "if len(raw) == 0 {", "new": "if len(items) == 0 {"}}`

	call, ok := ToolCallInText(text, theTools)

	require.True(t, ok)
	require.Equal(t, "edit_file", call.Name)
	require.Equal(t, "plugins/mempalace/inject.go", call.Arguments["path"])
	require.Equal(t, "if len(items) == 0 {", call.Arguments["new"])
}

// Ollama's own shape, written out by hand.
func TestTheNestedFunctionShapeIsRecoveredToo(t *testing.T) {
	text := `{"function": {"name": "run_command", "arguments": {"command": "go test ./..."}}}`

	call, ok := ToolCallInText(text, theTools)

	require.True(t, ok)
	require.Equal(t, "run_command", call.Name)
	require.Equal(t, "go test ./...", call.Arguments["command"])
}

// Everything before the last call is a model explaining what it could do, and
// running an illustration is not what it asked for.
func TestOnlyTheLastCallIsTaken(t *testing.T) {
	text := `First I could read it:
{"name": "read_file", "arguments": {"path": "a.go"}}
but what I will actually do is:
{"name": "write_file", "arguments": {"path": "b.go", "content": "package b"}}`

	call, ok := ToolCallInText(text, theTools)

	require.True(t, ok)
	require.Equal(t, "write_file", call.Name)
}

// Braces inside a Go snippet must not end the object early.
func TestAGoSnippetInAnArgumentDoesNotBreakTheParse(t *testing.T) {
	text := `{"name": "write_file", "arguments": {"path": "m.go", "content": "func main() {\n\tif x {\n\t\ty()\n\t}\n}"}}`

	call, ok := ToolCallInText(text, theTools)

	require.True(t, ok)
	require.Contains(t, call.Arguments["content"], "if x {")
}

func TestAToolNobodyOffersIsNotRecovered(t *testing.T) {
	text := `{"name": "delete_everything", "arguments": {"path": "/"}}`

	_, ok := ToolCallInText(text, theTools)

	require.False(t, ok)
}

func TestOrdinaryProseIsNotACall(t *testing.T) {
	for _, text := range []string{
		"",
		"I would use read_file to look at it.",
		"Here is a map literal: {\"a\": 1}",
		"{not json at all",
	} {
		_, ok := ToolCallInText(text, theTools)
		require.False(t, ok, "for %q", text)
	}
}

func TestWithNoToolsNothingIsRecovered(t *testing.T) {
	_, ok := ToolCallInText(`{"name": "read_file", "arguments": {}}`, nil)

	require.False(t, ok)
}
