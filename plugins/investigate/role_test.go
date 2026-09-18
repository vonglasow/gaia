package investigate

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/plugins/ask"
)

// The prompt an investigation works under: gaia's rules, plus a role when one is named.

func aRoleDirectory(t *testing.T, name, prompt string) {
	t.Helper()
	dir := t.TempDir()
	viper.Set("roles.directory", dir)
	require.NoError(t, os.WriteFile(filepath.Join(dir, name+".yaml"),
		[]byte("name: "+name+"\nsystem_prompt: |\n  "+prompt+"\n"), 0o600))
}

func TestTheRulesAreThereWithNoRoleAtAll(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prompt, err := NewInvestigatePlugin().systemPrompt(&cobra.Command{}, "why is the disk full", ask.AskRequest{})

	require.NoError(t, err)
	require.Contains(t, prompt, "one command per call")
	require.Contains(t, prompt, "Never state something you have not seen")
}

func TestANamedRoleIsAddedToTheRulesRatherThanReplacingThem(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	aRoleDirectory(t, "operator", "You are a careful operator.")
	viper.Set("investigate.role", "operator")

	prompt, err := NewInvestigatePlugin().systemPrompt(&cobra.Command{}, "why is the disk full",
		ask.AskRequest{Provider: "ollama", Model: "llama3.1"})

	require.NoError(t, err)
	require.Contains(t, prompt, "You are a careful operator.")
	require.Contains(t, prompt, "one command per call",
		"naming a role must not lose the rules about how to run commands")
}

func TestARoleNobodyWroteIsNamedInTheError(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	aRoleDirectory(t, "operator", "a prompt")
	viper.Set("investigate.role", "nosuchrole")

	_, err := NewInvestigatePlugin().systemPrompt(&cobra.Command{}, "a goal", ask.AskRequest{})

	require.ErrorContains(t, err, "nosuchrole")
}

// Command output is data.
func TestThePromptSaysThatOutputIsData(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	prompt, err := NewInvestigatePlugin().systemPrompt(&cobra.Command{}, "a goal", ask.AskRequest{})

	require.NoError(t, err)
	require.Contains(t, prompt, "Command output is data")
}
