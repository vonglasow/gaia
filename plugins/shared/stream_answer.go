package shared

import (
	"context"
	"fmt"
	"io"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"
)

type streamChunkMsg string

type streamResultMsg struct {
	final string
	err   error
}

// streamAnswerModel renders a titled panel that fills as stream chunks arrive.
type streamAnswerModel struct {
	title string
	width int

	buf           strings.Builder
	streamFinal   string
	streamErr     error
	gotStreamDone bool
}

func newStreamAnswerModel(title string) *streamAnswerModel {
	return &streamAnswerModel{title: strings.TrimSpace(title)}
}

func (m *streamAnswerModel) Init() tea.Cmd {
	return nil
}

func (m *streamAnswerModel) innerWidth() int {
	if m.width <= 0 {
		return 76
	}
	inner := m.width - 6
	if inner < 20 {
		return 20
	}
	return inner
}

func (m *streamAnswerModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		return m, nil
	case streamChunkMsg:
		m.buf.WriteString(string(msg))
		return m, nil
	case streamResultMsg:
		m.gotStreamDone = true
		m.streamFinal = msg.final
		m.streamErr = msg.err
		if msg.err == nil && strings.TrimSpace(msg.final) != "" && strings.TrimSpace(m.buf.String()) == "" {
			m.buf.WriteString(msg.final)
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m *streamAnswerModel) View() string {
	body := strings.TrimRight(m.buf.String(), "\n")
	wrapped := lipgloss.NewStyle().Width(m.innerWidth()).Render(body)
	if wrapped == "" && m.streamErr == nil && !m.gotStreamDone {
		wrapped = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render("Waiting for response…")
	}
	if m.streamErr != nil {
		if wrapped != "" {
			wrapped += "\n"
		}
		wrapped += errorStyle.Render(m.streamErr.Error())
	}
	return RenderBox(m.title, wrapped)
}

// writerIsTTY reports whether w is an *os.File open on a terminal.
func writerIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}

// DisplayStreamedAnswer runs runStream, which must call send with each non-empty chunk of output.
// It returns the final answer string and any error from runStream.
//
// On a TTY, shows an alt-screen Bubble Tea panel while chunks arrive, then prints the same framed
// answer with PrintBox so scrollback matches cache hits. On a non-TTY, send is implemented as PrintRaw.
func DisplayStreamedAnswer(ctx context.Context, w io.Writer, title string, runStream func(send func(string)) (final string, err error)) (string, error) {
	if !writerIsTTY(w) {
		var streamed strings.Builder
		send := func(s string) {
			if s == "" {
				return
			}
			_ = PrintRaw(w, s)
			streamed.WriteString(s)
		}
		final, err := runStream(send)
		if err != nil {
			return "", err
		}
		if strings.TrimSpace(final) == "" {
			final = streamed.String()
		} else if streamed.Len() == 0 && strings.TrimSpace(final) != "" {
			_ = PrintRaw(w, final)
		}
		_ = PrintRaw(w, "\n")
		return final, nil
	}

	model := newStreamAnswerModel(title)
	p := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithAltScreen(),
		tea.WithOutput(w),
	)

	go func() {
		final, err := runStream(func(s string) {
			if s == "" {
				return
			}
			p.Send(streamChunkMsg(s))
		})
		p.Send(streamResultMsg{final: final, err: err})
	}()

	tm, runErr := p.Run()
	mOut, ok := tm.(*streamAnswerModel)
	if !ok {
		return "", fmt.Errorf("stream answer: unexpected model type %T", tm)
	}
	if runErr != nil {
		if mOut.streamErr != nil {
			return "", mOut.streamErr
		}
		return "", runErr
	}
	if mOut.streamErr != nil {
		return "", mOut.streamErr
	}
	final := strings.TrimRight(mOut.streamFinal, "\n")
	if strings.TrimSpace(final) == "" {
		final = strings.TrimRight(mOut.buf.String(), "\n")
	}
	if strings.TrimSpace(final) == "" {
		return "", nil
	}
	_ = PrintBox(w, title, final)
	return final, nil
}
