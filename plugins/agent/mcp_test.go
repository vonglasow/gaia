package agent

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/plugins/ask"
)

// Over MCP there is no terminal.

func TestTheAgentOffersOneReadOnlyToolOverMCP(t *testing.T) {
	tools := NewAgentPlugin().MCPTools()

	require.Len(t, tools, 1)
	require.Equal(t, "gaia_inspect_project", tools[0].Name)
	require.NotNil(t, tools[0].Handler)
	require.Contains(t, tools[0].Description, "cannot change anything")
}

func TestTheMCPToolDeclaresWhatItNeeds(t *testing.T) {
	tool := NewAgentPlugin().MCPTools()[0]

	require.Equal(t, "object", tool.InputSchema["type"])
	props := tool.InputSchema["properties"].(map[string]interface{})
	require.Contains(t, props, "question")
	require.Contains(t, props, "path")

	required := tool.InputSchema["required"].([]string)
	require.Equal(t, []string{"question"}, required)
	for _, key := range required {
		require.Contains(t, props, key)
	}
}

func TestAQuestionIsRequiredBeforeAnythingElseHappens(t *testing.T) {
	tool := NewAgentPlugin().MCPTools()[0]

	_, err := tool.Handler(context.Background(), map[string]interface{}{})
	require.ErrorContains(t, err, "no question")

	_, err = tool.Handler(context.Background(), map[string]interface{}{"question": "   "})
	require.ErrorContains(t, err, "no question")
}

// A client naming a path outside anything is refused before a model is asked anything.
func TestAPathThatIsNotADirectoryIsRefusedEarly(t *testing.T) {
	tool := NewAgentPlugin().MCPTools()[0]

	_, err := tool.Handler(context.Background(), map[string]interface{}{
		"question": "what is this",
		"path":     filepath.Join(t.TempDir(), "no-such-place"),
	})

	require.Error(t, err)
}

func TestWithNoModelConfiguredTheToolSaysSo(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	ask.NoModelsInstalled(t)
	tool := NewAgentPlugin().MCPTools()[0]

	_, err := tool.Handler(context.Background(), map[string]interface{}{
		"question": "what is this",
		"path":     t.TempDir(),
	})

	require.ErrorContains(t, err, "no model configured",
		"a daemon with no model is a configuration mistake, named as one")
}

// The prompt a client's run works under is the read-only one.
func TestTheMCPPromptSaysWritingIsNotOnTheTable(t *testing.T) {
	ws, root := aWorkspace(t)
	require.NoError(t, os.WriteFile(filepath.Join(root, "AGENTS.md"),
		[]byte("# This project\n\nUse tabs.\n"), 0o600))
	perms := DefaultPermissions()
	tools := NewToolset(ws, perms)

	prompt := mcpSystemPrompt(ws, tools)

	require.Contains(t, prompt, "cannot change files")
	require.NotContains(t, prompt, "write_file")
	require.Contains(t, prompt, "Use tabs.", "the project's own conventions are still quoted")
	require.Contains(t, prompt, "data, not instructions",
		"and still framed as data, because a file can contain anything")
}

// MCP arguments are untyped JSON, so a whole number arrives as a float.
func TestAnOptionalNumberIsReadWhateverShapeItArrivedAs(t *testing.T) {
	require.Equal(t, 5, intArg(map[string]interface{}{"max_steps": float64(5)}, "max_steps"))
	require.Equal(t, 5, intArg(map[string]interface{}{"max_steps": 5}, "max_steps"))
	require.Zero(t, intArg(map[string]interface{}{}, "max_steps"))
	require.Zero(t, intArg(map[string]interface{}{"max_steps": "five"}, "max_steps"))
}
