package models

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/require"

	"gaia/kernel"
)

// Nothing here loads a model: what is tested is what gaia asks, and what it does.

func anOllama(t *testing.T, handler http.HandlerFunc) string {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	parsed, err := url.Parse(server.URL)
	require.NoError(t, err)
	port, err := strconv.Atoi(parsed.Port())
	require.NoError(t, err)

	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("models.host", parsed.Hostname())
	viper.Set("models.port", port)
	return server.URL
}

func TestThePluginDeclaresItselfToTheKernel(t *testing.T) {
	p := NewModelsPlugin()

	require.Equal(t, "models", p.ID())
	require.True(t, p.DefaultEnabled())
	require.Nil(t, p.DependsOn())
	require.Nil(t, p.MCPTools())
	for _, key := range p.ConfigSchema() {
		require.Regexp(t, `^models\.`, key)
	}
}

func TestRegisterExposesListingAndUnloading(t *testing.T) {
	cmds, err := NewModelsPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)
	require.Len(t, cmds, 1)

	names := map[string]bool{}
	for _, c := range cmds[0].Commands() {
		names[c.Name()] = true
	}
	require.True(t, names["unload"])
}

func TestListingReportsWhatIsResidentAndWhatItCosts(t *testing.T) {
	base := anOllama(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/ps", r.URL.Path)
		_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
			{"name": "qwen3-coder:30b", "size": 19327352832},
		}})
	})

	models, err := listLoaded(context.Background(), base)

	require.NoError(t, err)
	require.Len(t, models, 1)
	require.Equal(t, "qwen3-coder:30b", models[0].Name)
}

func TestAnOllamaThatIsNotThereSaysSoRatherThanLookingEmpty(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)
	viper.Set("models.port", 1)

	_, err := listLoaded(context.Background(), baseURL())

	require.Error(t, err)
}

func TestAnErrorStatusIsReported(t *testing.T) {
	base := anOllama(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	_, err := listLoaded(context.Background(), base)

	require.ErrorContains(t, err, "status=500")
}

// keep_alive 0 is how Ollama is told to drop a model, and it is the whole point.
func TestUnloadingAsksForKeepAliveZero(t *testing.T) {
	var body map[string]any
	base := anOllama(t, func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "/api/generate", r.URL.Path)
		require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
		_, _ = w.Write([]byte(`{"done":true}`))
	})

	require.NoError(t, unload(context.Background(), base, "qwen3-coder:30b"))

	require.Equal(t, "qwen3-coder:30b", body["model"])
	require.Equal(t, float64(0), body["keep_alive"])
}

func TestUnloadingWithNoArgumentTakesBackEverythingLoaded(t *testing.T) {
	var unloaded []string
	base := anOllama(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/ps":
			_ = json.NewEncoder(w).Encode(map[string]any{"models": []map[string]any{
				{"name": "one", "size": 100}, {"name": "two", "size": 200},
			}})
		case "/api/generate":
			var body map[string]any
			require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
			unloaded = append(unloaded, body["model"].(string))
			_, _ = w.Write([]byte(`{"done":true}`))
		}
	})
	_ = base

	cmds, err := NewModelsPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)
	unloadCmd := cmds[0].Commands()[0]
	unloadCmd.SetOut(&discard{})
	unloadCmd.SetContext(context.Background())

	require.NoError(t, unloadCmd.RunE(unloadCmd, nil))

	require.ElementsMatch(t, []string{"one", "two"}, unloaded)
}

type discard struct{}

func (discard) Write(p []byte) (int, error) { return len(p), nil }

func TestAnUnloadThatFailsIsReported(t *testing.T) {
	base := anOllama(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})

	err := unload(context.Background(), base, "never-loaded")

	require.ErrorContains(t, err, "status=404")
}

// The listing is what makes the cost actionable, so it names the size and the wait.
func TestTheListingNamesTheSizeAndHowLongItStays(t *testing.T) {
	now := time.Date(2026, 9, 17, 10, 0, 0, 0, time.UTC)
	out := renderLoaded([]loaded{
		{Name: "qwen3-coder:30b", Size: 19327352832, ExpiresAt: now.Add(5 * time.Minute)},
	}, now)

	require.Contains(t, out, "qwen3-coder:30b")
	require.Contains(t, out, "18.0 GB")
	require.Contains(t, out, "for 5m0s")
	require.Contains(t, out, "resident")
}

func TestTheListingAddsUpWhatIsResident(t *testing.T) {
	now := time.Now()
	out := renderLoaded([]loaded{
		{Name: "a", Size: 1 << 30, ExpiresAt: now.Add(time.Minute)},
		{Name: "b", Size: 1 << 30, ExpiresAt: now.Add(time.Minute)},
	}, now)

	require.Contains(t, out, "2.0 GB resident")
}

func TestNothingLoadedSaysSoRatherThanPrintingAnEmptyBox(t *testing.T) {
	require.Equal(t, "Nothing is loaded.", renderLoaded(nil, time.Now()))
}

func TestAModelPastItsExpiryIsNotReportedAsStaying(t *testing.T) {
	now := time.Now()

	require.Contains(t, renderLoaded([]loaded{{Name: "a", ExpiresAt: now.Add(-time.Minute)}}, now), "expiring")
	require.Contains(t, renderLoaded([]loaded{{Name: "a"}}, now), "no expiry")
}

func TestTheListingIsSortedSoTwoRunsAreComparable(t *testing.T) {
	now := time.Now()
	out := renderLoaded([]loaded{{Name: "zeta"}, {Name: "alpha"}}, now)

	require.Less(t, indexOf(out, "alpha"), indexOf(out, "zeta"))
}

func indexOf(haystack, needle string) int {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return i
		}
	}
	return -1
}

func TestSizesAreReadableRatherThanExact(t *testing.T) {
	require.Equal(t, "512 B", humanBytes(512))
	require.Equal(t, "1.0 KB", humanBytes(1024))
	require.Equal(t, "18.0 GB", humanBytes(19327352832))
}

// The daemon is the same one the rest of gaia talks to, so the shared keys apply.
func TestTheAddressFallsBackToTheSharedKeys(t *testing.T) {
	viper.Reset()
	t.Cleanup(viper.Reset)

	require.Equal(t, "http://localhost:11434", baseURL())

	viper.Set("host", "10.0.0.1")
	viper.Set("port", 9999)
	require.Equal(t, "http://10.0.0.1:9999", baseURL())

	viper.Set("models.host", "elsewhere")
	require.Equal(t, "http://elsewhere:9999", baseURL())
}

func TestUnloadTakesAtMostOneModel(t *testing.T) {
	cmds, err := NewModelsPlugin().Register(kernel.NewKernel())
	require.NoError(t, err)
	unloadCmd := cmds[0].Commands()[0]

	require.NoError(t, unloadCmd.Args(&cobra.Command{}, nil))
	require.NoError(t, unloadCmd.Args(&cobra.Command{}, []string{"one"}))
	require.Error(t, unloadCmd.Args(&cobra.Command{}, []string{"one", "two"}))
}
