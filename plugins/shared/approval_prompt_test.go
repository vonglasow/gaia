package shared

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/stretchr/testify/require"
)

// This is the prompt standing between a suggested command and the machine running it.

func key(s string) tea.KeyMsg {
	switch s {
	case "enter":
		return tea.KeyMsg{Type: tea.KeyEnter}
	case "ctrl+c":
		return tea.KeyMsg{Type: tea.KeyCtrlC}
	case "space":
		return tea.KeyMsg{Type: tea.KeySpace}
	default:
		return tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune(s)}
	}
}

// press feeds one key and hands back the model in its new state.
func press(m tea.Model, s string) approvalPromptModel {
	next, _ := m.Update(key(s))
	return next.(approvalPromptModel)
}

func TestChoosingAllowExactEndsTheProptWithThatDecision(t *testing.T) {
	m := press(newApprovalPromptModel("git status", "git *", "git status"), "1")

	require.True(t, m.done)
	require.False(t, m.cancelled)
	require.Equal(t, "allow_exact", m.result.Decision)
	require.Empty(t, m.result.Pattern, "an exact decision carries no pattern")
}

func TestChoosingDenyExactEndsThePromptWithThatDecision(t *testing.T) {
	m := press(newApprovalPromptModel("rm -rf /", "rm *", "rm -rf /"), "3")

	require.True(t, m.done)
	require.Equal(t, "deny_exact", m.result.Decision)
}

// A pattern choice does not decide anything yet.
func TestChoosingAPatternAsksForItBeforeDeciding(t *testing.T) {
	m := press(newApprovalPromptModel("git status", "git *", "git status"), "2")

	require.False(t, m.done, "nothing is decided until the pattern is given")
	require.Equal(t, stepPattern, m.step)
	require.Equal(t, "git *", m.input.Placeholder)
}

func TestAPatternTypedInIsTheOneRecorded(t *testing.T) {
	m := press(newApprovalPromptModel("git status", "git *", "git status"), "2")
	m.input.SetValue("  git log *  ")

	m = press(m, "enter")

	require.True(t, m.done)
	require.Equal(t, "allow_pattern", m.result.Decision)
	require.Equal(t, "git log *", m.result.Pattern, "the surrounding spaces are not part of the pattern")
}

// Pressing enter on an empty field is how a person accepts the suggestion.
func TestAnEmptyPatternFallsBackToTheSuggestedOne(t *testing.T) {
	m := press(newApprovalPromptModel("git status", "git *", "git status"), "4")
	m = press(m, "enter")

	require.Equal(t, "deny_pattern", m.result.Decision)
	require.Equal(t, "git *", m.result.Pattern)
}

func TestEditingOffersTheCommandAndKeepsWhatWasTyped(t *testing.T) {
	m := press(newApprovalPromptModel("git psuh", "git *", "git psuh"), "5")
	require.Equal(t, stepEdit, m.step)
	require.Equal(t, "git psuh", m.input.Placeholder)

	m.input.SetValue("git push")
	m = press(m, "enter")

	require.Equal(t, "edit", m.result.Decision)
	require.Equal(t, "git push", m.result.NewKey)
}

func TestAnEmptyEditKeepsTheCommandAsItWas(t *testing.T) {
	m := press(newApprovalPromptModel("git status", "git *", "git status"), "5")
	m = press(m, "enter")

	require.Equal(t, "edit", m.result.Decision)
	require.Equal(t, "git status", m.result.NewKey)
}

// Cancelling must be reachable by more than one key.
func TestEveryWayOutCancelsWithoutDeciding(t *testing.T) {
	for _, k := range []string{"6", "q", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			m := press(newApprovalPromptModel("rm -rf /", "rm *", "rm -rf /"), k)

			require.True(t, m.done)
			require.True(t, m.cancelled)
			require.Empty(t, m.result.Decision)
		})
	}
}

func TestCancellingFromThePatternFieldDecidesNothingEither(t *testing.T) {
	m := press(newApprovalPromptModel("rm -rf /", "rm *", "rm -rf /"), "2")
	m = press(m, "ctrl+c")

	require.True(t, m.cancelled)
	require.Empty(t, m.result.Decision)
}

// Once the prompt is done it must stop reacting.
func TestAKeyArrivingAfterTheDecisionChangesNothing(t *testing.T) {
	m := press(newApprovalPromptModel("git status", "git *", "git status"), "1")
	m = press(m, "3")

	require.Equal(t, "allow_exact", m.result.Decision)
}

func TestAKeyNobodyMappedIsIgnored(t *testing.T) {
	m := press(newApprovalPromptModel("git status", "git *", "git status"), "z")

	require.False(t, m.done)
	require.Empty(t, m.result.Decision)
}

func TestTheWindowSizeIsTakenAndNotTreatedAsADecision(t *testing.T) {
	next, _ := newApprovalPromptModel("git status", "git *", "git status").
		Update(tea.WindowSizeMsg{Width: 120})
	m := next.(approvalPromptModel)

	require.Equal(t, 120, m.width)
	require.False(t, m.done)

	next, _ = m.Update(tea.WindowSizeMsg{Width: 0})
	require.Equal(t, 120, next.(approvalPromptModel).width,
		"a width of zero is a terminal that has not reported yet, not a terminal of no width")
}

func TestTheApprovalViewNamesTheCommandAndTheWayOut(t *testing.T) {
	view := newApprovalPromptModel("git status", "git *", "git status").View()

	require.Contains(t, view, "git status")
	require.NotEmpty(t, view)
}

func TestInitAsksForNothing(t *testing.T) {
	require.Nil(t, newApprovalPromptModel("git status", "git *", "git status").Init())
	require.Nil(t, newConfirmationPromptModel("proceed?", "").Init())
	require.Nil(t, newCommandPreviewModel("git status", "").Init())
}

// --- the yes/no prompt ----------------------------------------------------

func TestConfirmationAcceptsOnYesAndOnEnter(t *testing.T) {
	for _, k := range []string{"y", "enter"} {
		t.Run(k, func(t *testing.T) {
			next, _ := newConfirmationPromptModel("run it?", "").Update(key(k))
			m := next.(confirmationPromptModel)

			require.True(t, m.done)
			require.True(t, m.confirmed)
		})
	}
}

func TestConfirmationRefusesOnEveryWayOut(t *testing.T) {
	for _, k := range []string{"n", "q", "ctrl+c"} {
		t.Run(k, func(t *testing.T) {
			next, _ := newConfirmationPromptModel("run it?", "").Update(key(k))
			m := next.(confirmationPromptModel)

			require.True(t, m.done)
			require.False(t, m.confirmed, "anything but yes is no")
		})
	}
}

func TestConfirmationIgnoresAKeyNobodyMapped(t *testing.T) {
	next, _ := newConfirmationPromptModel("run it?", "").Update(key("z"))

	require.False(t, next.(confirmationPromptModel).done,
		"an unmapped key must not be read as consent")
}

func TestConfirmationCarriesADefaultTitle(t *testing.T) {
	require.Equal(t, "Confirm", newConfirmationPromptModel("run it?", "").title)
	require.Equal(t, "Danger", newConfirmationPromptModel("run it?", "Danger").title)
	require.Contains(t, newConfirmationPromptModel("run it?", "").View(), "run it?")
}

// --- the command preview --------------------------------------------------

func TestThePreviewRunsOnEnterOrOnAnyOrdinaryKey(t *testing.T) {
	for _, k := range []string{"enter", "space", "a"} {
		t.Run(k, func(t *testing.T) {
			next, _ := newCommandPreviewModel("git status", "").Update(key(k))
			m := next.(commandPreviewModel)

			require.True(t, m.done)
			require.False(t, m.canceled)
			require.False(t, m.skipped)
		})
	}
}

func TestThePreviewSkipsOnSAndQuitsOnQ(t *testing.T) {
	next, _ := newCommandPreviewModel("git status", "").Update(key("s"))
	require.True(t, next.(commandPreviewModel).skipped)

	next, _ = newCommandPreviewModel("git status", "").Update(key("q"))
	require.True(t, next.(commandPreviewModel).canceled)

	next, _ = newCommandPreviewModel("git status", "").Update(key("ctrl+c"))
	require.True(t, next.(commandPreviewModel).canceled)
}

func TestThePreviewShowsTheCommandAndItsChoices(t *testing.T) {
	view := newCommandPreviewModel("git status", "").View()

	require.Contains(t, view, "git status")
	require.Contains(t, view, "skip")
}
