package shared

import (
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/charmbracelet/bubbles/progress"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const progressPadding = 2

var progressHelpStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("#626262")).Render

type ProgressUpdate struct {
	Completed int64
	Total     int64
}

// ProgressDone signals the progress UI to finish.
type ProgressDone struct{}

// ProgressModel manages the download progress UI.
type ProgressModel struct {
	progress  progress.Model
	Total     int64
	Completed int64
	Done      bool
	Canceled  bool
	OnCancel  func()
	mutex     sync.Mutex
}

func NewProgressModel(width int) *ProgressModel {
	if width <= 0 {
		width = 50
	}
	return &ProgressModel{progress: progress.New(progress.WithWidth(width))}
}

func (m *ProgressModel) Init() tea.Cmd {
	return nil
}

func (m *ProgressModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ProgressDone:
		m.Done = true
		return m, tea.Quit
	case tea.KeyMsg:
		if msg.String() == "q" || msg.String() == "ctrl+c" {
			m.Canceled = true
			if m.OnCancel != nil {
				m.OnCancel()
			}
			return m, tea.Quit
		}
	case ProgressUpdate:
		m.mutex.Lock()
		m.Completed, m.Total = msg.Completed, msg.Total
		m.mutex.Unlock()
		if m.Total > 0 {
			m.progress.SetPercent(float64(m.Completed) / float64(m.Total))
		}
	}
	return m, nil
}

func (m *ProgressModel) View() string {
	if m.Done {
		return "\nDownload completed! Proceeding...\n"
	}
	progressPercent := float64(0)
	if m.Total > 0 {
		progressPercent = float64(m.Completed) / float64(m.Total)
	}
	pad := strings.Repeat(" ", progressPadding)
	return "\n" +
		pad + m.progress.ViewAs(progressPercent) + "\n\n" +
		pad + progressHelpStyle("Press 'q' to cancel")
}

func NewProgressProgram(model *ProgressModel, out io.Writer) *tea.Program {
	if out == nil {
		out = io.Discard
	}
	return tea.NewProgram(model, tea.WithOutput(out), tea.WithInput(nil))
}

type ProgressClearer struct {
	mu      sync.Mutex
	pending bool
}

func (p *ProgressClearer) MarkPending() {
	if p == nil {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pending = true
}

func (p *ProgressClearer) ClearOnce(w io.Writer) {
	if p == nil {
		return
	}
	p.mu.Lock()
	if !p.pending {
		p.mu.Unlock()
		return
	}
	p.pending = false
	p.mu.Unlock()
	_, _ = fmt.Fprint(w, "\r\033[2K")
}
