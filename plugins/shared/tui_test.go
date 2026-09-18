package shared

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// failingWriter answers an error to every write.
type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, errors.New("pipe closed") }

func TestRenderBoxPutsTheTitleAboveTheBody(t *testing.T) {
	out := RenderBox("Config", "ask.model: llama3.1")

	require.Contains(t, out, "Config")
	require.Contains(t, out, "ask.model: llama3.1")
	require.Contains(t, out, "╭", "the box is drawn, not merely indented")
}

// An empty body would collapse the box into something read as a crash.
func TestRenderBoxSaysWhenThereIsNothingToShow(t *testing.T) {
	require.Contains(t, RenderBox("Roles", ""), "(empty)")
	require.Contains(t, RenderBox("", ""), "(empty)")
}

func TestRenderBoxWithoutATitleStillDrawsABox(t *testing.T) {
	out := RenderBox("", "just a body")

	require.Contains(t, out, "just a body")
	require.Contains(t, out, "╭")
}

func TestPrintBoxWritesWhereItIsPointed(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, PrintBox(&out, "Version", "Gaia v1"))
	require.Contains(t, out.String(), "Gaia v1")
	require.True(t, strings.HasSuffix(out.String(), "\n"))
}

func TestPrintBoxReportsAWriteThatFailed(t *testing.T) {
	require.Error(t, PrintBox(failingWriter{}, "Version", "Gaia v1"))
}

// PrintError returns whether the *write* succeeded, not whether the program did.
func TestPrintErrorReturnsTheWriteResultAndNotAFailure(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, PrintError(&out, "something went wrong"),
		"a message printed successfully is, to this function, a success")
	require.Contains(t, out.String(), "something went wrong")

	require.Error(t, PrintError(failingWriter{}, "something went wrong"),
		"the only error it can report is its own")
}

func TestPrintErrorSaysNothingAboutAnEmptyMessage(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, PrintError(&out, "   "))
	require.Empty(t, out.String())
	require.NoError(t, PrintError(failingWriter{}, ""),
		"there was nothing to write, so there was nothing to fail")
}

func TestPrintPromptLeavesTheCursorOnTheSameLine(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, PrintPrompt(&out, "You: "))
	require.Contains(t, out.String(), "You: ")
	require.False(t, strings.HasSuffix(out.String(), "\n"),
		"a prompt the person types after must not end the line")
}

func TestPrintRawAddsNothingOfItsOwn(t *testing.T) {
	var out bytes.Buffer

	require.NoError(t, PrintRaw(&out, "exact text"))
	require.Equal(t, "exact text", out.String())

	require.NoError(t, PrintRaw(failingWriter{}, ""), "nothing to write cannot fail")
}

func TestRenderProgressBarFillsInProportion(t *testing.T) {
	require.Equal(t, "[----------]", RenderProgressBar(0, 100, 10))
	require.Equal(t, "[#####-----]", RenderProgressBar(50, 100, 10))
	require.Equal(t, "[##########]", RenderProgressBar(100, 100, 10))
}

// A download that reports more bytes than it announced.
func TestRenderProgressBarClampsWhatItIsGiven(t *testing.T) {
	require.Equal(t, "[##########]", RenderProgressBar(500, 100, 10))
	require.Equal(t, "[----------]", RenderProgressBar(-5, 100, 10))
}

func TestRenderProgressBarDrawsNothingWithoutATotalOrAWidth(t *testing.T) {
	require.Equal(t, "", RenderProgressBar(5, 0, 10))
	require.Equal(t, "", RenderProgressBar(5, 100, 0))
}
