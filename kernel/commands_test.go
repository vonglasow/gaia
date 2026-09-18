package kernel

import (
	"bytes"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

// A group command with no Run of its own used to print its help and exit 0 for any.
func TestAGroupRefusesAnArgumentItDoesNotKnow(t *testing.T) {
	root := &cobra.Command{Use: "gaia"}
	group := &cobra.Command{Use: "config"}
	group.AddCommand(&cobra.Command{Use: "get", RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(group)

	refuseUnknownSubcommands(root)

	err := group.RunE(group, []string{"nosuchthing"})

	require.ErrorContains(t, err, `unknown command "nosuchthing"`)
}

// No argument at all is somebody asking what the group holds.
func TestAGroupWithNoArgumentStillShowsItsHelp(t *testing.T) {
	root := &cobra.Command{Use: "gaia"}
	group := &cobra.Command{Use: "config"}
	group.AddCommand(&cobra.Command{Use: "get", RunE: func(*cobra.Command, []string) error { return nil }})
	root.AddCommand(group)
	var out bytes.Buffer
	group.SetOut(&out)

	refuseUnknownSubcommands(root)
	require.NoError(t, group.RunE(group, nil))

	require.Contains(t, out.String(), "get")
}

// A command that does its own work keeps doing it.
func TestACommandThatRunsIsLeftAlone(t *testing.T) {
	ran := false
	root := &cobra.Command{Use: "gaia"}
	leaf := &cobra.Command{Use: "version", RunE: func(*cobra.Command, []string) error {
		ran = true
		return nil
	}}
	root.AddCommand(leaf)

	refuseUnknownSubcommands(root)
	require.NoError(t, leaf.RunE(leaf, nil))

	require.True(t, ran)
}

// Nested groups are reached too.
func TestNestedGroupsAreCoveredAsWell(t *testing.T) {
	root := &cobra.Command{Use: "gaia"}
	outer := &cobra.Command{Use: "outer"}
	inner := &cobra.Command{Use: "inner"}
	inner.AddCommand(&cobra.Command{Use: "leaf", RunE: func(*cobra.Command, []string) error { return nil }})
	outer.AddCommand(inner)
	root.AddCommand(outer)

	refuseUnknownSubcommands(root)

	require.ErrorContains(t, inner.RunE(inner, []string{"nope"}), "unknown command")
}
