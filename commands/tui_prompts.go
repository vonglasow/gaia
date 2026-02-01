package commands

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	contextMaxLines = 20
	defaultWidth    = 80
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4")).
			MarginBottom(1)

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#626262")).
			Padding(0, 1).
			Margin(0, 0, 1, 0)

	optionStyle = lipgloss.NewStyle().
			Foreground(lipgloss.Color("#A0A0A0"))

	keyStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))
)

// contextPromptModel is the Bubble Tea model for the context prompt (tool/operator).
type contextPromptModel struct {
	initialContext string
	input          textinput.Model
	done           bool
	result         string
	cancelled      bool
	width          int
}

func newContextPromptModel(initialContext string) contextPromptModel {
	ti := textinput.New()
	ti.Placeholder = "Enter to use as-is • text to replace • +text to append • q to quit"
	ti.PromptStyle = keyStyle
	ti.TextStyle = lipgloss.NewStyle()
	ti.Focus()
	ti.CharLimit = 4096
	return contextPromptModel{
		initialContext: initialContext,
		input:          ti,
		width:          defaultWidth,
	}
}

func (m contextPromptModel) Init() tea.Cmd {
	return textinput.Blink
}

func (m contextPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.done {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c":
			m.cancelled = true
			m.done = true
			return m, tea.Quit
		case "q":
			// Quit only when input is empty; otherwise let "q" be typed
			if strings.TrimSpace(m.input.Value()) == "" {
				m.cancelled = true
				m.done = true
				return m, tea.Quit
			}
		case "enter":
			m.done = true
			input := strings.TrimSpace(m.input.Value())
			if input == "q" || input == "quit" {
				m.cancelled = true
				return m, tea.Quit
			}
			if input == "" {
				m.result = m.initialContext
				return m, tea.Quit
			}
			if strings.HasPrefix(input, "+") {
				appendText := strings.TrimSpace(strings.TrimPrefix(input, "+"))
				if m.initialContext != "" {
					m.result = m.initialContext + "\n\n" + appendText
				} else {
					m.result = appendText
				}
			} else {
				m.result = input
			}
			return m, tea.Quit
		}
	}

	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m contextPromptModel) View() string {
	// Constrain to terminal width (border + padding = 2 each side)
	contentWidth := m.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	wrapStyle := lipgloss.NewStyle().Width(contentWidth)

	// Context section
	ctxTitle := titleStyle.Render("Current Context")
	ctxBody := "(no context)"
	if m.initialContext != "" {
		lines := strings.Split(m.initialContext, "\n")
		if len(lines) > contextMaxLines {
			ctxBody = strings.Join(lines[:contextMaxLines], "\n") +
				fmt.Sprintf("\n... (%d more lines)", len(lines)-contextMaxLines)
		} else {
			ctxBody = m.initialContext
		}
	}
	ctxBox := boxStyle.Width(contentWidth).Render(wrapStyle.Render(ctxBody))

	// Options
	opts := strings.Join([]string{
		keyStyle.Render("Enter") + " use as-is",
		keyStyle.Render("text") + " replace",
		keyStyle.Render("+text") + " append",
		keyStyle.Render("q") + " quit",
	}, "  •  ")
	optsLine := optionStyle.Width(contentWidth + 4).Render(opts)

	return wrapStyle.Width(m.width).Render(ctxTitle + "\n" + ctxBox + "\n" + optsLine + "\n\n" + m.input.View())
}

// confirmationPromptModel is the Bubble Tea model for yes/no confirmation (tool/operator).
type confirmationPromptModel struct {
	message   string
	title     string
	done      bool
	confirmed bool
	width     int
}

func newConfirmationPromptModel(message, title string) confirmationPromptModel {
	if title == "" {
		title = "Confirm"
	}
	return confirmationPromptModel{message: message, title: title, width: defaultWidth}
}

func (m confirmationPromptModel) Init() tea.Cmd {
	return nil
}

func (m confirmationPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.done {
		return m, nil
	}

	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		if msg.Width > 0 {
			m.width = msg.Width
		}
		return m, nil
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "n", "q":
			m.confirmed = false
			m.done = true
			return m, tea.Quit
		case "enter", "y":
			m.confirmed = true
			m.done = true
			return m, tea.Quit
		}
	}

	return m, nil
}

func (m confirmationPromptModel) View() string {
	contentWidth := m.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	wrapStyle := lipgloss.NewStyle().Width(contentWidth)

	msgTitle := titleStyle.Render(m.title)
	msgLines := strings.Split(m.message, "\n")
	maxLines := 25
	if len(msgLines) > maxLines {
		msgLines = append(msgLines[:maxLines], fmt.Sprintf("... (%d more lines)", len(msgLines)-maxLines))
	}
	msgBody := strings.Join(msgLines, "\n")
	msgBox := boxStyle.Width(contentWidth).Render(wrapStyle.Render(msgBody))

	opts := keyStyle.Render("y") + "/" + keyStyle.Render("Enter") + " confirm  " +
		keyStyle.Render("n") + " cancel"
	optsLine := optionStyle.Render(opts)

	return lipgloss.NewStyle().Width(m.width).Render(msgTitle + "\n" + msgBox + "\n\n" + optsLine)
}

// runContextPromptTUI runs the context prompt TUI and returns the selected context or error.
func runContextPromptTUI(initialContext string) (string, error) {
	model := newContextPromptModel(initialContext)
	p := tea.NewProgram(model, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return "", err
	}
	m, ok := final.(contextPromptModel)
	if !ok {
		return "", nil
	}
	if m.cancelled {
		return "", fmt.Errorf("cancelled by user")
	}
	return m.result, nil
}

// runConfirmationPromptTUI runs the confirmation TUI and returns true if confirmed.
func runConfirmationPromptTUI(message, title string) (bool, error) {
	model := newConfirmationPromptModel(message, title)
	p := tea.NewProgram(model, tea.WithAltScreen())
	final, err := p.Run()
	if err != nil {
		return false, err
	}
	m, ok := final.(confirmationPromptModel)
	if !ok {
		return false, nil
	}
	return m.confirmed, nil
}
