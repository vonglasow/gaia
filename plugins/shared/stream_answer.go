package shared

import (
	"context"
	"fmt"
	"io"
	"os"
	"strconv"
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

func newStreamAnswerModel(title string, width int) *streamAnswerModel {
	return &streamAnswerModel{
		title: strings.TrimSpace(title),
		width: width,
	}
}

func (m *streamAnswerModel) Init() tea.Cmd {
	return nil
}

func (m *streamAnswerModel) innerWidth() int {
	if m.width <= 2 {
		return 0
	}
	return m.width - 2
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
		// Finalize model content before quitting so the last frame is complete.
		// Prefer the authoritative final payload when provided.
		if msg.err == nil {
			final := strings.TrimRight(msg.final, "\n")
			if strings.TrimSpace(final) == "" {
				final = strings.TrimRight(m.buf.String(), "\n")
			}
			if strings.TrimSpace(final) != "" {
				m.buf.Reset()
				m.buf.WriteString(final)
			}
		}
		return m, tea.Quit
	}
	return m, nil
}

func (m *streamAnswerModel) View() string {
	if m.width <= 0 {
		// Do not render until we know terminal width.
		return ""
	}

	body := strings.TrimRight(m.buf.String(), "\n")
	wrapped := lipgloss.NewStyle().Width(m.innerWidth()).Render(body)
	if wrapped == "" && m.streamErr == nil && !m.gotStreamDone {
		wrapped = "Waiting for response..."
	}
	if m.streamErr != nil {
		if wrapped != "" {
			wrapped += "\n"
		}
		wrapped += m.streamErr.Error()
	}
	rendered := renderFixedWidthBox(m.title, wrapped, m.width)
	if m.gotStreamDone {
		// Keep cursor on a line below the border so Bubble Tea exit cleanup
		// clears that line, not the bottom border itself.
		return rendered + "\n"
	}
	return rendered
}

// writerIsTTY reports whether w is an *os.File open on a terminal.
func writerIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(f.Fd())
}

var detectTTY = writerIsTTY

func writerTerminalWidth(w io.Writer) (int, bool) {
	_ = w
	// Bubble Tea's WindowSizeMsg is the source of truth for dynamic sizing.
	// COLUMNS is only used as an optional initial hint before the first resize event.
	columns := strings.TrimSpace(os.Getenv("COLUMNS"))
	if columns == "" {
		return 0, false
	}
	width, err := strconv.Atoi(columns)
	if err != nil || width <= 0 {
		return 0, false
	}
	return width, true
}

var detectTerminalWidth = writerTerminalWidth

func renderFixedWidthBox(title, body string, width int) string {
	if width < 2 {
		return ""
	}
	inner := width - 2
	lines := make([]string, 0, 8)
	if strings.TrimSpace(title) != "" {
		lines = append(lines, strings.TrimSpace(title))
	}
	body = strings.TrimRight(body, "\n")
	if body == "" {
		body = "(empty)"
	}
	lines = append(lines, strings.Split(body, "\n")...)

	top := "╭" + strings.Repeat("─", inner) + "╮"
	bottom := "╰" + strings.Repeat("─", inner) + "╯"
	framed := make([]string, 0, len(lines)+2)
	framed = append(framed, top)
	for _, line := range lines {
		trimmed := strings.ReplaceAll(line, "\r", "")
		w := lipgloss.Width(trimmed)
		if w > inner {
			// lipgloss wrapping should avoid this, but guard against overflow.
			runes := []rune(trimmed)
			if inner < len(runes) {
				trimmed = string(runes[:inner])
				w = lipgloss.Width(trimmed)
			}
		}
		padding := inner - w
		if padding < 0 {
			padding = 0
		}
		framed = append(framed, "│"+trimmed+strings.Repeat(" ", padding)+"│")
	}
	framed = append(framed, bottom)
	return strings.Join(framed, "\n")
}

// DisplayStreamedAnswer runs runStream, which must call send with each non-empty chunk of output.
// It returns the final answer string and any error from runStream.
//
// On a TTY, Bubble Tea is the single renderer and writes in normal terminal flow (no alt-screen).
// After exit we print exactly one newline so the shell prompt does not overwrite the bottom border.
// On a non-TTY, send is implemented as PrintRaw.
func DisplayStreamedAnswer(ctx context.Context, w io.Writer, title string, runStream func(send func(string)) (final string, err error)) (string, error) {
	if !detectTTY(w) {
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

	initialWidth, _ := detectTerminalWidth(w)
	model := newStreamAnswerModel(title, initialWidth)
	p := tea.NewProgram(
		model,
		tea.WithContext(ctx),
		tea.WithOutput(w),
		tea.WithInput(nil),
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
	_ = PrintRaw(w, "\n")
	return final, nil
}
