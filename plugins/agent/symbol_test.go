package agent

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Reading a file to change one function costs the whole file, every turn it
// stays in the conversation.

const aSourceFile = `package thing

import "fmt"

// Answer is what the thing is for.
const Answer = 42

// Thing does something.
type Thing struct {
	Name string
}

// Describe says what a thing is.
func (t *Thing) Describe() string {
	return fmt.Sprintf("a thing called %s", t.Name)
}

// Build makes one.
//
// It takes a name and gives it back wrapped.
func Build(name string) *Thing {
	return &Thing{Name: name}
}

func unexported() {}
`

func TestAFunctionIsReadWithTheCommentAboveIt(t *testing.T) {
	got, line, err := findSymbol(aSourceFile, "Build")

	require.NoError(t, err)
	require.Contains(t, got, "// Build makes one.")
	require.Contains(t, got, "It takes a name and gives it back wrapped.")
	require.Contains(t, got, "return &Thing{Name: name}")
	require.NotContains(t, got, "func unexported", "and stops where it ends")
	require.Positive(t, line)
}

// The lines are numbered the way read_file numbers them, so a quote copied from
// either can be handed straight to edit_file.
func TestWhatIsReadIsNumberedLikeAFileRead(t *testing.T) {
	got, line, err := findSymbol(aSourceFile, "Build")

	require.NoError(t, err)
	require.True(t, strings.HasPrefix(got, "18\t// Build makes one."), "got %q", strings.Split(got, "\n")[0])
	require.Equal(t, 18, line)
}

func TestAMethodIsAskedForByItsType(t *testing.T) {
	got, _, err := findSymbol(aSourceFile, "Thing.Describe")

	require.NoError(t, err)
	require.Contains(t, got, "func (t *Thing) Describe() string")
	require.NotContains(t, got, "func Build")
}

func TestATypeAndAConstantAreReadToo(t *testing.T) {
	typeSrc, _, err := findSymbol(aSourceFile, "Thing")
	require.NoError(t, err)
	require.Contains(t, typeSrc, "Name string")

	constSrc, _, err := findSymbol(aSourceFile, "Answer")
	require.NoError(t, err)
	require.Contains(t, constSrc, "Answer = 42")
}

func TestAnUnexportedFunctionIsReadAsWell(t *testing.T) {
	got, _, err := findSymbol(aSourceFile, "unexported")

	require.NoError(t, err)
	require.Contains(t, got, "func unexported()")
}

func TestANameThatIsNotThereSaysSo(t *testing.T) {
	_, _, err := findSymbol(aSourceFile, "NoSuchThing")

	require.ErrorContains(t, err, `there is no "NoSuchThing" in this file`)
}

func TestAFileThatIsNotGoSaysThatInstead(t *testing.T) {
	_, _, err := findSymbol("this is not go at all", "Build")

	require.ErrorContains(t, err, "cannot read this file as Go")
}

// --- through the toolset ---------------------------------------------------

func TestReadingASymbolThroughTheToolset(t *testing.T) {
	ts, root := aProject(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "thing.go"), []byte(aSourceFile), 0o600))

	out := ts.Call(context.Background(), call("read_symbol",
		map[string]any{"path": "thing.go", "name": "Build"}))

	require.Contains(t, out, "func Build(name string) *Thing")
	require.NotContains(t, out, "func unexported")
}

func TestReadingASymbolOutsideTheWorkspaceIsRefused(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("read_symbol",
		map[string]any{"path": "../elsewhere.go", "name": "Build"}))

	require.Contains(t, out, "outside the workspace")
}

func TestReadingASymbolFromAFileThatIsNotThereSaysSo(t *testing.T) {
	ts, _ := aProject(t)

	out := ts.Call(context.Background(), call("read_symbol",
		map[string]any{"path": "nope.go", "name": "Build"}))

	require.Contains(t, out, "no such file")
}

// It is offered without --write: reading is not changing.
func TestReadingASymbolIsOfferedToAReadOnlyRun(t *testing.T) {
	ts, _ := aProject(t)

	require.Contains(t, ts.Names(), "read_symbol")
}
