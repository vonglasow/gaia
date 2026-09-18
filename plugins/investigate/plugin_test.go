package investigate

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
	"gaia/plugins/ask"
)

// investigate is now the agent's loop with one tool.

func TestThePluginDeclaresItselfToTheKernel(t *testing.T) {
	p := NewInvestigatePlugin()

	require.Equal(t, "investigate", p.ID())
	require.True(t, p.DefaultEnabled())
	require.Nil(t, p.DependsOn())

	for _, key := range p.ConfigSchema() {
		require.Regexp(t, `^investigate\.`, key, "%q is outside the plugin's namespace", key)
	}
}

// Nothing is exposed over MCP.
func TestInvestigateIsNotOnTheMCPSurface(t *testing.T) {
	require.Nil(t, NewInvestigatePlugin().MCPTools())
}

// confirm_medium_risk is gone.
func TestConfirmationIsNoLongerASetting(t *testing.T) {
	for _, key := range NewInvestigatePlugin().ConfigSchema() {
		require.NotEqual(t, "investigate.confirm_medium_risk", key)
	}
}

// A machine configured for `gaia ask` can run `gaia investigate` without a second copy.
func TestTheModelIsResolvedFromTheSharedKeysWhenInvestigateHasNone(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "llama3.1")

	req, err := NewInvestigatePlugin().request(investigateCommand(t))

	require.NoError(t, err)
	require.Equal(t, "llama3.1", req.Model)
	require.Equal(t, "localhost", req.Host, "an investigation talks to the local model by default")
	require.Equal(t, 11434, req.Port)
	require.Equal(t, "ollama", req.Provider, "inferred from the model when nobody named one")
}

func TestTheInvestigateKeysWinOverTheSharedOnes(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "shared-model")
	viper.Set("investigate.model", "investigate-model")

	req, err := NewInvestigatePlugin().request(investigateCommand(t))

	require.NoError(t, err)
	require.Equal(t, "investigate-model", req.Model)
}

func TestWithNoModelAnywhereItSaysWhatIsMissing(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	ask.NoModelsInstalled(t)

	_, err := NewInvestigatePlugin().request(investigateCommand(t))

	require.ErrorContains(t, err, "model")
}

func TestRegisterExposesTheFlagsTheCommandDocuments(t *testing.T) {
	cmds, err := NewInvestigatePlugin().Register(kernel.NewKernel())
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	for _, flag := range []string{"max-steps", "yes", "role", "model", "quiet"} {
		require.NotNilf(t, cmds[0].Flags().Lookup(flag), "flag --%s is missing", flag)
	}
}

// investigateCommand is a command carrying the flags request() reads.
func investigateCommand(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{}
	cmd.Flags().String("model", "", "")
	return cmd
}
