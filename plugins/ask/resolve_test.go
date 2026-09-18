package ask

import (
	"testing"
	"time"

	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"
)

// Written once because it was written four times, and the fifth would have
// differed. These are the rules every command now shares.

func TestWhatWasTypedWinsOverEverything(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("agent.model", "configured")
	viper.Set("model", "shared")
	viper.Set("agent.provider", "ollama")

	req, err := Endpoint{Plugin: "agent", Model: "typed", Provider: "mistral"}.Resolve()

	require.NoError(t, err)
	require.Equal(t, "typed", req.Model)
	require.Equal(t, "mistral", req.Provider)
}

func TestThePluginKeyWinsOverTheSharedOne(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "shared")
	viper.Set("agent.model", "agent-model")

	req, err := Endpoint{Plugin: "agent"}.Resolve()

	require.NoError(t, err)
	require.Equal(t, "agent-model", req.Model)
}

// A machine set up for `gaia ask` runs `gaia agent` without a second copy of
// the same three keys.
func TestTheSharedKeysAreEnoughOnTheirOwn(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "llama3.1")

	req, err := Endpoint{Plugin: "agent"}.Resolve()

	require.NoError(t, err)
	require.Equal(t, "llama3.1", req.Model)
}

// A local model is the sensible default rather than a decision to make first.
func TestTheLocalOllamaIsTheDefaultAddress(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "llama3.1")

	req, err := Endpoint{Plugin: "agent"}.Resolve()

	require.NoError(t, err)
	require.Equal(t, "localhost", req.Host)
	require.Equal(t, 11434, req.Port)
}

func TestTheProviderIsInferredFromTheModelWhenNobodyNamedOne(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	viper.Set("model", "gpt-4o")
	req, err := Endpoint{Plugin: "ask"}.Resolve()
	require.NoError(t, err)
	require.Equal(t, "openai", req.Provider)

	viper.Set("model", "llama3.1")
	req, err = Endpoint{Plugin: "ask"}.Resolve()
	require.NoError(t, err)
	require.Equal(t, "ollama", req.Provider)
}

// A caller may ask for longer than a single question, and configuration still
// wins over that: an agent reasoning over a file takes longer than an answer.
func TestTheCallersTimeoutAppliesUnlessConfigured(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "llama3.1")

	req, err := Endpoint{Plugin: "agent", Timeout: 5 * time.Minute}.Resolve()
	require.NoError(t, err)
	require.Equal(t, 5*time.Minute, req.Timeout)

	viper.Set("agent.timeout_seconds", 30)
	req, err = Endpoint{Plugin: "agent", Timeout: 5 * time.Minute}.Resolve()
	require.NoError(t, err)
	require.Equal(t, 30*time.Second, req.Timeout)
}

func TestWithNoTimeoutAnywhereThereIsStillOne(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("model", "llama3.1")

	req, err := Endpoint{Plugin: "ask"}.Resolve()

	require.NoError(t, err)
	require.Equal(t, DefaultTimeout, req.Timeout)
}

// The message names both keys, because a person reading it is about to set one.
func TestWithNoModelAnywhereTheErrorNamesTheKeys(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	NoModelsInstalled(t)

	_, err := Endpoint{Plugin: "agent"}.Resolve()

	require.ErrorContains(t, err, "agent.model")
	require.ErrorContains(t, err, "--model")
}

func TestEachPluginReadsItsOwnNamespace(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("agent.model", "for-the-agent")
	viper.Set("investigate.model", "for-investigate")

	agent, err := Endpoint{Plugin: "agent"}.Resolve()
	require.NoError(t, err)
	investigate, err := Endpoint{Plugin: "investigate"}.Resolve()
	require.NoError(t, err)

	require.Equal(t, "for-the-agent", agent.Model)
	require.Equal(t, "for-investigate", investigate.Model)
}

// Nobody should have to name a model gaia could have found. Ollama knows what
// it holds, and what it is already holding costs nothing to use.

func TestAModelAlreadyLoadedIsTheOneUsed(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	ModelLoaded(t, "qwen2.5:latest")

	req, err := Endpoint{Plugin: "ask"}.Resolve()

	require.NoError(t, err)
	require.Equal(t, "qwen2.5:latest", req.Model)
	require.Equal(t, "ollama", req.Provider, "and the provider follows from it")
}

func TestWithOneModelInstalledItIsTheOneUsed(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	ModelsInstalled(t, "llama3.1:8b")

	req, err := Endpoint{Plugin: "ask"}.Resolve()

	require.NoError(t, err)
	require.Equal(t, "llama3.1:8b", req.Model)
}

// With several installed and none loaded there is a choice to make, and making
// it silently would be worse than asking.
func TestWithSeveralInstalledTheErrorNamesThem(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	ModelsInstalled(t, "llama3.1:8b", "qwen2.5:latest")

	_, err := Endpoint{Plugin: "ask"}.Resolve()

	require.ErrorContains(t, err, "no model configured")
	require.ErrorContains(t, err, "llama3.1:8b, qwen2.5:latest")
}

func TestWithOllamaUnreachableTheErrorSaysOnlyWhatToSet(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	NoModelsInstalled(t)

	_, err := Endpoint{Plugin: "ask"}.Resolve()

	require.ErrorContains(t, err, "ask.model")
	require.NotContains(t, err.Error(), "installed here")
}

// What was configured still wins: finding one is a fallback, not a preference.
func TestAConfiguredModelIsNotOverriddenByWhatIsLoaded(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	ModelLoaded(t, "qwen2.5:latest")
	viper.Set("model", "llama3.1:8b")

	req, err := Endpoint{Plugin: "ask"}.Resolve()

	require.NoError(t, err)
	require.Equal(t, "llama3.1:8b", req.Model)
}
