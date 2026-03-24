package shared

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

func TestStreamAnswerModel_ChunksAndResult(t *testing.T) {
	m := newStreamAnswerModel("Answer")
	m.width = 80

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
	m := newStreamAnswerModel("Answer")
	m.width = 80
	next, cmd := m.Update(streamResultMsg{final: "only final", err: nil})
	require.NotNil(t, cmd)
	m = next.(*streamAnswerModel)
	require.Equal(t, "only final", m.buf.String())
	require.Equal(t, "only final", m.streamFinal)
}

func TestStreamAnswerModel_Error(t *testing.T) {
	m := newStreamAnswerModel("Answer")
	m.width = 80
	next, cmd := m.Update(streamResultMsg{final: "", err: tea.ErrInterrupted})
	require.NotNil(t, cmd)
	m = next.(*streamAnswerModel)
	require.ErrorIs(t, m.streamErr, tea.ErrInterrupted)
}
