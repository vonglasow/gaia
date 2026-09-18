package chat

import (
	"fmt"
	"sort"
	"strings"

	"gaia/plugins/ask"
	"gaia/plugins/roles"
)

// session is what one conversation carries between turns.
type session struct {
	// role is the specialisation in force, or "" for whatever auto-selection picks.
	role string
	// history is every turn so far, which is what makes this a conversation.
	history []ask.ChatMessage
}

// outcome is what a slash command did.
type outcome struct {
	// handled says the line was a command, so it never reaches the model.
	handled bool
	// reply is what to show the person.
	reply string
	// quit ends the session.
	quit bool
}

// knownRoles is replaced in tests, so the commands can be driven without a roles directory.
var knownRoles = func() (map[string]roles.ResolvedRole, error) {
	loaded, err := roles.LoadRolesWithDefaults()
	if err != nil {
		return nil, err
	}
	return roles.ResolveInheritance(loaded)
}

// run applies a line to the session, and reports whether it was a command.
//
// A specialisation that can only be chosen at launch means restarting to ask a
// different kind of question, and losing the conversation to do it.
func (s *session) run(line string) outcome {
	line = strings.TrimSpace(line)
	if strings.EqualFold(line, "exit") {
		return outcome{handled: true, quit: true, reply: "Chat session ended."}
	}
	if !strings.HasPrefix(line, "/") {
		return outcome{}
	}

	command, argument, _ := strings.Cut(strings.TrimPrefix(line, "/"), " ")
	argument = strings.TrimSpace(argument)

	switch strings.ToLower(command) {
	case "exit", "quit":
		return outcome{handled: true, quit: true, reply: "Chat session ended."}
	case "help":
		return outcome{handled: true, reply: helpText()}
	case "roles":
		return outcome{handled: true, reply: s.listRoles()}
	case "role":
		return outcome{handled: true, reply: s.setRole(argument)}
	case "reset":
		s.history = nil
		return outcome{handled: true, reply: "History cleared. The role is unchanged."}
	default:
		return outcome{handled: true, reply: fmt.Sprintf("There is no /%s. Try /help.", command)}
	}
}

// setRole changes the specialisation, or reports the one in force.
func (s *session) setRole(name string) string {
	if name == "" {
		if s.role == "" {
			return "No role is set; one is picked per question if auto-selection is on."
		}
		return "Role: " + s.role
	}
	if strings.EqualFold(name, "none") || strings.EqualFold(name, "off") {
		s.role = ""
		return "Role cleared."
	}

	resolved, err := knownRoles()
	if err != nil {
		return "Roles could not be read: " + err.Error()
	}
	if _, ok := resolved[name]; !ok {
		return fmt.Sprintf("There is no role called %q. Try /roles.", name)
	}
	s.role = name
	// The history stays: the point of switching mid-session is to carry the
	// conversation into a different specialisation.
	return "Role: " + name
}

func (s *session) listRoles() string {
	resolved, err := knownRoles()
	if err != nil {
		return "Roles could not be read: " + err.Error()
	}
	if len(resolved) == 0 {
		return "No roles are available."
	}
	names := make([]string, 0, len(resolved))
	for name := range resolved {
		names = append(names, name)
	}
	sort.Strings(names)

	var b strings.Builder
	for _, name := range names {
		marker := "  "
		if name == s.role {
			marker = "* "
		}
		b.WriteString(marker + name)
		if description := strings.TrimSpace(resolved[name].Description); description != "" {
			b.WriteString("  — " + description)
		}
		b.WriteString("\n")
	}
	return strings.TrimRight(b.String(), "\n")
}

func helpText() string {
	return strings.Join([]string{
		"/role            show the role in force",
		"/role <name>     work under that role from now on",
		"/role none       stop using one",
		"/roles           list what is available",
		"/reset           forget the conversation, keep the role",
		"/help            this",
		"/exit            leave",
	}, "\n")
}
