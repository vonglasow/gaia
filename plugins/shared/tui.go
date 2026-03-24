package shared

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

var (
	titleStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))
	errorStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#FF5F5F"))
	promptStyle = lipgloss.NewStyle().
			Bold(true).
			Foreground(lipgloss.Color("#7D56F4"))
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#626262")).
			Padding(0, 1)
)

// RenderBox formats a title + body into a styled box.
func RenderBox(title, body string) string {
	title = strings.TrimSpace(title)
	body = strings.TrimRight(body, "\n")
	if body == "" {
		body = "(empty)"
	}
	if title != "" {
		title = titleStyle.Render(title)
		return boxStyle.Render(title + "\n" + body)
	}
	return boxStyle.Render(body)
}

// PrintBox writes a styled box to the writer.
func PrintBox(w io.Writer, title, body string) error {
	_, err := fmt.Fprintln(w, RenderBox(title, body))
	return err
}

// PrintError writes a styled error message.
func PrintError(w io.Writer, message string) error {
	if strings.TrimSpace(message) == "" {
		return nil
	}
	_, err := fmt.Fprintln(w, errorStyle.Render(message))
	return err
}

// PrintPrompt writes a styled prompt label (no trailing newline).
func PrintPrompt(w io.Writer, label string) error {
	_, err := fmt.Fprint(w, promptStyle.Render(label))
	return err
}

// PrintRaw writes raw text without styling.
func PrintRaw(w io.Writer, text string) error {
	if text == "" {
		return nil
	}
	_, err := fmt.Fprint(w, text)
	return err
}

// RenderProgressBar renders a simple ASCII progress bar.
func RenderProgressBar(current, total, width int) string {
	if total <= 0 || width <= 0 {
		return ""
	}
	if current < 0 {
		current = 0
	}
	if current > total {
		current = total
	}
	filled := int(float64(current) / float64(total) * float64(width))
	if filled < 0 {
		filled = 0
	}
	if filled > width {
		filled = width
	}
	return "[" + strings.Repeat("#", filled) + strings.Repeat("-", width-filled) + "]"
}
