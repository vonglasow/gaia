package shared

import (
	"bytes"
	"context"
	"io"
	"regexp"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/require"
)

func TestStreamAnswerModel_ChunksAndResult(t *testing.T) {
	m := newStreamAnswerModel("Answer", 80)

	var next tea.Model
	var cmd tea.Cmd
	next, cmd = m.Update(tea.WindowSizeMsg{Width: 100, Height: 40})
	require.Nil(t, cmd)
	m = next.(*streamAnswerModel)
	require.Equal(t, 100, m.width)

	next, cmd = m.Update(streamChunkMsg("hello"))
	require.Nil(t, cmd)
	m = next.(*streamAnswerModel)
	require.Equal(t, "hello", m.buf.String())

	next, cmd = m.Update(streamResultMsg{final: "hello", err: nil})
	require.NotNil(t, cmd)
	m = next.(*streamAnswerModel)
	require.True(t, m.gotStreamDone)
	require.NoError(t, m.streamErr)
	require.Equal(t, "hello", m.streamFinal)
}

func TestStreamAnswerModel_ResultFillsEmptyBuffer(t *testing.T) {
	m := newStreamAnswerModel("Answer", 80)
	next, cmd := m.Update(streamResultMsg{final: "only final", err: nil})
	require.NotNil(t, cmd)
	m = next.(*streamAnswerModel)
	require.Equal(t, "only final", m.buf.String())
	require.Equal(t, "only final", m.streamFinal)
}

func TestStreamAnswerModel_ResultFinalizesBufferWithAuthoritativeFinal(t *testing.T) {
	m := newStreamAnswerModel("Answer", 80)
	next, _ := m.Update(streamChunkMsg("partial"))
	m = next.(*streamAnswerModel)
	next, cmd := m.Update(streamResultMsg{final: "partial + final tail", err: nil})
	require.NotNil(t, cmd)
	m = next.(*streamAnswerModel)
	require.Equal(t, "partial + final tail", m.buf.String())
}

func TestStreamAnswerModel_Error(t *testing.T) {
	m := newStreamAnswerModel("Answer", 80)
	next, cmd := m.Update(streamResultMsg{final: "", err: tea.ErrInterrupted})
	require.NotNil(t, cmd)
	m = next.(*streamAnswerModel)
	require.ErrorIs(t, m.streamErr, tea.ErrInterrupted)
}

func TestDisplayStreamedAnswer_TTYStreamKeepsFinalFrameAndNewline(t *testing.T) {
	prevDetectTTY := detectTTY
	detectTTY = func(_ io.Writer) bool { return true }
	t.Cleanup(func() { detectTTY = prevDetectTTY })
	prevDetectWidth := detectTerminalWidth
	detectTerminalWidth = func(_ io.Writer) (int, bool) { return 60, true }
	t.Cleanup(func() { detectTerminalWidth = prevDetectWidth })

	var out bytes.Buffer
	final, err := DisplayStreamedAnswer(context.Background(), &out, "Answer", func(send func(string)) (string, error) {
		send("hello")
		return "hello", nil
	})
	require.NoError(t, err)
	require.Equal(t, "hello", final)

	rendered := out.String()
	// Must preserve a full framed answer.
	require.Contains(t, rendered, "╭")
	require.Contains(t, rendered, "╰")
	require.Contains(t, rendered, "Answer")
	require.Contains(t, rendered, "hello")
	// Must leave the cursor on the next line so shell prompt won't overwrite the bottom border.
	require.True(t, strings.HasSuffix(rendered, "\n"))
}

func TestStreamAnswerModel_TerminalWidthExact(t *testing.T) {
	for _, width := range []int{40, 60, 120} {
		m := newStreamAnswerModel("Answer", width)
		next, _ := m.Update(streamChunkMsg("hello world this line is intentionally long to wrap"))
		m = next.(*streamAnswerModel)
		view := m.View()
		require.NotEmpty(t, view)
		for _, line := range strings.Split(strings.TrimRight(view, "\n"), "\n") {
			require.Equal(t, width, lipgloss.Width(line))
		}
	}
}

func TestStreamAnswerModel_SingleBox(t *testing.T) {
	m := newStreamAnswerModel("Answer", 60)
	next, _ := m.Update(streamChunkMsg("hello"))
	m = next.(*streamAnswerModel)
	view := m.View()
	require.Equal(t, 1, strings.Count(view, "╭"))
	require.Equal(t, 1, strings.Count(view, "╰"))
}

func TestDisplayStreamedAnswer_NoDuplication(t *testing.T) {
	prevDetectTTY := detectTTY
	detectTTY = func(_ io.Writer) bool { return true }
	t.Cleanup(func() { detectTTY = prevDetectTTY })
	prevDetectWidth := detectTerminalWidth
	detectTerminalWidth = func(_ io.Writer) (int, bool) { return 60, true }
	t.Cleanup(func() { detectTerminalWidth = prevDetectWidth })

	var out bytes.Buffer
	_, err := DisplayStreamedAnswer(context.Background(), &out, "Answer", func(send func(string)) (string, error) {
		send("hello world")
		return "hello world", nil
	})
	require.NoError(t, err)
	clean := stripANSIAndCR(out.String())
	require.Equal(t, 1, strings.Count(clean, "hello world"))
	require.Equal(t, 1, strings.Count(clean, "╭"))
	require.Equal(t, 1, strings.Count(clean, "╰"))
}

func TestDisplayStreamedAnswer_TTYSimulationUsesProvidedWidth(t *testing.T) {
	prevDetectTTY := detectTTY
	detectTTY = func(_ io.Writer) bool { return true }
	t.Cleanup(func() { detectTTY = prevDetectTTY })
	prevDetectWidth := detectTerminalWidth
	detectTerminalWidth = func(_ io.Writer) (int, bool) { return 40, true }
	t.Cleanup(func() { detectTerminalWidth = prevDetectWidth })

	var out bytes.Buffer
	_, err := DisplayStreamedAnswer(context.Background(), &out, "Answer", func(send func(string)) (string, error) {
		send("short")
		return "short", nil
	})
	require.NoError(t, err)

	clean := stripANSIAndCR(out.String())
	lines := strings.Split(strings.TrimRight(clean, "\n"), "\n")
	require.GreaterOrEqual(t, len(lines), 3)
	require.Equal(t, 40, lipgloss.Width(lines[0]))
}

func TestDisplayStreamedAnswer_FinalLineIntegrity(t *testing.T) {
	prevDetectTTY := detectTTY
	detectTTY = func(_ io.Writer) bool { return true }
	t.Cleanup(func() { detectTTY = prevDetectTTY })
	prevDetectWidth := detectTerminalWidth
	detectTerminalWidth = func(_ io.Writer) (int, bool) { return 60, true }
	t.Cleanup(func() { detectTerminalWidth = prevDetectWidth })

	var out bytes.Buffer
	_, err := DisplayStreamedAnswer(context.Background(), &out, "Answer", func(send func(string)) (string, error) {
		send("hello")
		return "hello", nil
	})
	require.NoError(t, err)

	raw := out.String()
	require.True(t, strings.HasSuffix(raw, "\n"))
	clean := stripANSIAndCR(raw)
	lines := strings.Split(strings.TrimRight(clean, "\n"), "\n")
	require.NotEmpty(t, lines)
	require.True(t, strings.HasPrefix(lines[len(lines)-1], "╰"))
}

func stripANSIAndCR(s string) string {
	ansi := regexp.MustCompile(`\x1b\[[0-9;?]*[A-Za-z]`)
	s = ansi.ReplaceAllString(s, "")
	s = strings.ReplaceAll(s, "\r", "")
	return s
}
