package chat

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"gaia/plugins/ask"
	"gaia/plugins/roles"
)

// A specialisation that can only be chosen at launch means restarting to ask a
// different kind of question, and losing the conversation to do it.

func withRoles(t *testing.T, available ...string) {
	t.Helper()
	previous := knownRoles
	t.Cleanup(func() { knownRoles = previous })
	knownRoles = func() (map[string]roles.ResolvedRole, error) {
		out := map[string]roles.ResolvedRole{}
		for _, name := range available {
			out[name] = roles.ResolvedRole{Name: name, Description: "a " + name}
		}
		return out, nil
	}
}

func TestAnOrdinaryLineIsAQuestionAndNotACommand(t *testing.T) {
	s := &session{}

	out := s.run("what is entanglement")

	require.False(t, out.handled, "it must reach the model")
	require.False(t, out.quit)
}

func TestExitEndsTheSessionWithOrWithoutASlash(t *testing.T) {
	for _, line := range []string{"exit", "/exit", "/quit", "EXIT"} {
		t.Run(line, func(t *testing.T) {
			out := (&session{}).run(line)
			require.True(t, out.handled)
			require.True(t, out.quit)
		})
	}
}

func TestTheRoleCanBeChangedMidSession(t *testing.T) {
	withRoles(t, "physics", "cook")
	s := &session{}

	out := s.run("/role physics")

	require.True(t, out.handled)
	require.Contains(t, out.reply, "physics")
	require.Equal(t, "physics", s.role)
}

// The conversation is the reason to switch rather than restart, so it stays.
func TestChangingRoleKeepsTheConversation(t *testing.T) {
	withRoles(t, "physics", "cook")
	s := &session{
		role:    "cook",
		history: []ask.ChatMessage{{Role: "user", Content: "earlier"}},
	}

	s.run("/role physics")

	require.Equal(t, "physics", s.role)
	require.Len(t, s.history, 1, "switching specialisation is not starting over")
}

func TestARoleNobodyWroteIsRefusedAndTheOldOneStands(t *testing.T) {
	withRoles(t, "physics")
	s := &session{role: "physics"}

	out := s.run("/role nope")

	require.Contains(t, out.reply, `no role called "nope"`)
	require.Contains(t, out.reply, "/roles", "and says where to look")
	require.Equal(t, "physics", s.role)
}

func TestAskingForTheRoleReportsIt(t *testing.T) {
	s := &session{role: "physics"}

	require.Contains(t, s.run("/role").reply, "physics")
}

func TestWithNoRoleSetItSaysWhatHappensInstead(t *testing.T) {
	s := &session{}

	require.Contains(t, s.run("/role").reply, "auto-selection")
}

func TestARoleCanBeTakenOffAgain(t *testing.T) {
	for _, word := range []string{"none", "off", "NONE"} {
		t.Run(word, func(t *testing.T) {
			s := &session{role: "physics"}
			out := s.run("/role " + word)
			require.Contains(t, out.reply, "cleared")
			require.Empty(t, s.role)
		})
	}
}

func TestTheListingNamesEveryRoleAndMarksTheActiveOne(t *testing.T) {
	withRoles(t, "physics", "cook")
	s := &session{role: "physics"}

	reply := s.run("/roles").reply

	require.Contains(t, reply, "physics")
	require.Contains(t, reply, "cook")
	require.Contains(t, reply, "* physics", "the one in force is marked")
	require.Contains(t, reply, "a cook", "and each carries its description")
}

func TestTheListingIsSortedSoTwoRunsAreComparable(t *testing.T) {
	withRoles(t, "zeta", "alpha")

	reply := (&session{}).run("/roles").reply

	require.Less(t, indexOf(reply, "alpha"), indexOf(reply, "zeta"))
}

func TestARolesDirectoryThatCannotBeReadIsReportedNotHidden(t *testing.T) {
	previous := knownRoles
	t.Cleanup(func() { knownRoles = previous })
	knownRoles = func() (map[string]roles.ResolvedRole, error) {
		return nil, errors.New("directory is not there")
	}
	s := &session{}

	require.Contains(t, s.run("/roles").reply, "directory is not there")
	require.Contains(t, s.run("/role physics").reply, "directory is not there")
	require.Empty(t, s.role, "a role is not set from a directory nobody could read")
}

func TestNoRolesAtAllSaysSoRatherThanPrintingNothing(t *testing.T) {
	withRoles(t)

	require.Contains(t, (&session{}).run("/roles").reply, "No roles")
}

// Resetting is for a new subject under the same specialisation.
func TestResettingForgetsTheConversationAndKeepsTheRole(t *testing.T) {
	s := &session{role: "physics", history: []ask.ChatMessage{{Role: "user", Content: "earlier"}}}

	out := s.run("/reset")

	require.Empty(t, s.history)
	require.Equal(t, "physics", s.role)
	require.Contains(t, out.reply, "role is unchanged")
}

func TestACommandNobodyWroteSaysWhereToLook(t *testing.T) {
	out := (&session{}).run("/nosuchthing")

	require.True(t, out.handled, "it is still a command, so it does not reach the model")
	require.Contains(t, out.reply, "/help")
}

func TestHelpNamesEveryCommand(t *testing.T) {
	reply := (&session{}).run("/help").reply

	for _, command := range []string{"/role", "/roles", "/reset", "/help", "/exit"} {
		require.Contains(t, reply, command)
	}
}

// A path typed by mistake must not be read as a command with a strange name.
func TestALineThatMerelyStartsWithASlashIsStillACommand(t *testing.T) {
	out := (&session{}).run("/etc/passwd is where accounts live")

	require.True(t, out.handled)
	require.Contains(t, out.reply, "/help")
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}
