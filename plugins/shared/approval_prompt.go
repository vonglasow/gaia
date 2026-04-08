package shared

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/bubbles/textinput"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

const (
	approvalDefaultWidth = 80
)

var (
	approvalTitleStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#7D56F4")).
				MarginBottom(1)

	approvalBoxStyle = lipgloss.NewStyle().
				Border(lipgloss.RoundedBorder()).
				BorderForeground(lipgloss.Color("#626262")).
				Padding(0, 1).
				Margin(0, 0, 1, 0)

	approvalOptionStyle = lipgloss.NewStyle().
				Foreground(lipgloss.Color("#A0A0A0"))

	approvalKeyStyle = lipgloss.NewStyle().
				Bold(true).
				Foreground(lipgloss.Color("#7D56F4"))
)

type approvalResult struct {
	Decision string
	Pattern  string
	NewKey   string
}

type approvalStep int

const (
	stepChoose approvalStep = iota
	stepPattern
	stepEdit
)

type approvalPromptModel struct {
	commandKey     string
	defaultPattern string
	defaultKey     string
	pendingAction  string
	step           approvalStep
	input          textinput.Model
	done           bool
	cancelled      bool
	result         approvalResult
	width          int
}

func newApprovalPromptModel(commandKey, defaultPattern, defaultKey string) approvalPromptModel {
	ti := textinput.New()
	ti.PromptStyle = approvalKeyStyle
	ti.TextStyle = lipgloss.NewStyle()
	ti.CharLimit = 4096
	return approvalPromptModel{
		commandKey:     commandKey,
		defaultPattern: defaultPattern,
		defaultKey:     defaultKey,
		input:          ti,
		width:          approvalDefaultWidth,
	}
}

func (m approvalPromptModel) Init() tea.Cmd {
	return nil
}

func (m approvalPromptModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
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
		switch m.step {
		case stepChoose:
			switch msg.String() {
			case "1":
				m.result = approvalResult{Decision: "allow_exact"}
				m.done = true
				return m, tea.Quit
			case "2":
				m.pendingAction = "allow_pattern"
				m.step = stepPattern
				m.input.Placeholder = m.defaultPattern
				m.input.SetValue("")
				m.input.Focus()
				return m, nil
			case "3":
				m.result = approvalResult{Decision: "deny_exact"}
				m.done = true
				return m, tea.Quit
			case "4":
				m.pendingAction = "deny_pattern"
				m.step = stepPattern
				m.input.Placeholder = m.defaultPattern
				m.input.SetValue("")
				m.input.Focus()
				return m, nil
			case "5":
				m.pendingAction = "edit"
				m.step = stepEdit
				m.input.Placeholder = m.defaultKey
				m.input.SetValue("")
				m.input.Focus()
				return m, nil
			case "6", "q", "ctrl+c":
				m.cancelled = true
				m.done = true
				return m, tea.Quit
			}
		case stepPattern, stepEdit:
			switch msg.String() {
			case "ctrl+c":
				m.cancelled = true
				m.done = true
				return m, tea.Quit
			case "enter":
				value := strings.TrimSpace(m.input.Value())
				if value == "" {
					if m.step == stepPattern {
						value = m.defaultPattern
					} else {
						value = m.defaultKey
					}
				}
				switch m.pendingAction {
				case "allow_pattern":
					m.result = approvalResult{Decision: "allow_pattern", Pattern: value}
				case "deny_pattern":
					m.result = approvalResult{Decision: "deny_pattern", Pattern: value}
				case "edit":
					m.result = approvalResult{Decision: "edit", NewKey: value}
				default:
					m.result = approvalResult{Decision: "cancel"}
				}
				m.done = true
				return m, tea.Quit
			}
		}
	}

	var cmd tea.Cmd
	if m.step == stepPattern || m.step == stepEdit {
		m.input, cmd = m.input.Update(msg)
		return m, cmd
	}
	return m, nil
}

func (m approvalPromptModel) View() string {
	contentWidth := m.width - 4
	if contentWidth < 20 {
		contentWidth = 20
	}
	wrapStyle := lipgloss.NewStyle().Width(contentWidth)

	title := approvalTitleStyle.Render("Approval")
	body := approvalBoxStyle.Width(contentWidth).Render(wrapStyle.Render(m.commandKey))

	if m.step == stepPattern {
		prompt := approvalOptionStyle.Render("Pattern (Enter to accept default)")
		return wrapStyle.Width(m.width).Render(title + "\n" + body + "\n" + prompt + "\n\n" + m.input.View())
	}
	if m.step == stepEdit {
		prompt := approvalOptionStyle.Render("Command key (Enter to accept default)")
		return wrapStyle.Width(m.width).Render(title + "\n" + body + "\n" + prompt + "\n\n" + m.input.View())
	}

	opts := strings.Join([]string{
		approvalKeyStyle.Render("1") + " allow exact",
		approvalKeyStyle.Render("2") + " allow pattern",
		approvalKeyStyle.Render("3") + " deny exact",
		approvalKeyStyle.Render("4") + " deny pattern",
		approvalKeyStyle.Render("5") + " edit",
		approvalKeyStyle.Render("6") + " cancel",
	}, "  •  ")
	optsLine := approvalOptionStyle.Width(contentWidth + 4).Render(opts)

	return wrapStyle.Width(m.width).Render(title + "\n" + body + "\n" + optsLine)
}

// RunApprovalPromptTUI shows a Bubble Tea prompt for tool approval.
func RunApprovalPromptTUI(commandKey, defaultPattern, defaultKey string, in io.Reader, out io.Writer) (string, string, string, error) {
	model := newApprovalPromptModel(commandKey, defaultPattern, defaultKey)
	prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithInput(in), tea.WithOutput(out))
	final, err := prog.Run()
	if err != nil {
		return "cancel", "", "", err
	}
	m, ok := final.(approvalPromptModel)
	if !ok {
		return "cancel", "", "", nil
	}
	if m.cancelled {
		return "cancel", "", "", fmt.Errorf("cancelled by user")
	}
	return m.result.Decision, m.result.Pattern, m.result.NewKey, nil
}

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
	return confirmationPromptModel{message: message, title: title, width: approvalDefaultWidth}
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

	msgTitle := approvalTitleStyle.Render(m.title)
	msgLines := strings.Split(m.message, "\n")
	maxLines := 25
	if len(msgLines) > maxLines {
		msgLines = append(msgLines[:maxLines], fmt.Sprintf("... (%d more lines)", len(msgLines)-maxLines))
	}
	msgBody := strings.Join(msgLines, "\n")
	msgBox := approvalBoxStyle.Width(contentWidth).Render(wrapStyle.Render(msgBody))

	opts := approvalKeyStyle.Render("y") + "/" + approvalKeyStyle.Render("Enter") + " confirm  " +
		approvalKeyStyle.Render("n") + " cancel"
	optsLine := approvalOptionStyle.Render(opts)

	return lipgloss.NewStyle().Width(m.width).Render(msgTitle + "\n" + msgBox + "\n\n" + optsLine)
}

// RunConfirmationPromptTUI shows a Bubble Tea confirmation prompt.
func RunConfirmationPromptTUI(message, title string, in io.Reader, out io.Writer) (bool, error) {
	model := newConfirmationPromptModel(message, title)
	prog := tea.NewProgram(model, tea.WithAltScreen(), tea.WithInput(in), tea.WithOutput(out))
	final, err := prog.Run()
	if err != nil {
		return false, err
	}
	m, ok := final.(confirmationPromptModel)
	if !ok {
		return false, nil
	}
	return m.confirmed, nil
}
