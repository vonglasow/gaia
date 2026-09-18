package ask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/spf13/viper"
)

// Endpoint resolves where a model lives, for any command that talks to one.
//
// Written once because it was written four times, and the fifth would have
// differed: the fallback chain, the defaults and the error have to say the
// same thing whether a person typed `gaia ask` or `gaia agent`.
type Endpoint struct {
	// Plugin namespaces the keys: "agent" reads agent.model, then model.
	Plugin string
	// Model and Provider are what was typed, and win over any configuration.
	Model    string
	Provider string
	// Timeout is how long one request may take. Zero takes the default.
	Timeout time.Duration
}

// DefaultTimeout is what a single question gets when nobody said.
const DefaultTimeout = 2 * time.Minute

// Resolve builds a request from flags, the plugin's own keys, and the shared ones.
func (e Endpoint) Resolve() (AskRequest, error) {
	key := func(name string) string { return e.Plugin + "." + name }

	req := AskRequest{
		Provider: FirstNonEmpty(e.Provider,
			FirstNonEmpty(viper.GetString(key("provider")), viper.GetString("provider"))),
		Model: FirstNonEmpty(e.Model,
			FirstNonEmpty(viper.GetString(key("model")), viper.GetString("model"))),
		Timeout: time.Duration(FirstNonZero(
			viper.GetInt(key("timeout_seconds")), viper.GetInt("timeout_seconds"))) * time.Second,
	}

	// A local model is the sensible default rather than a decision to make first.
	req.Host, req.Port = Address(e.Plugin)
	if req.Timeout == 0 {
		req.Timeout = e.Timeout
	}
	if req.Timeout == 0 {
		req.Timeout = DefaultTimeout
	}
	if strings.TrimSpace(req.Provider) == "" {
		req.Provider = ResolveProviderFromModel(req.Model)
	}
	if strings.TrimSpace(req.Model) == "" {
		picked, installed := pickModel(req.Host, req.Port)
		if picked == "" {
			return req, noModelError(key("model"), installed)
		}
		req.Model = picked
		req.Provider = FirstNonEmpty(req.Provider, ResolveProviderFromModel(picked))
	}
	return req, nil
}

// noModelError names what is installed: the next thing anyone needs is the
// spelling of a model that is actually there.
func noModelError(pluginKey string, installed []string) error {
	base := fmt.Sprintf("no model configured: set %s or model, or pass --model", pluginKey)
	if len(installed) == 0 {
		return errors.New(base)
	}
	return fmt.Errorf("%s\ninstalled here: %s", base, strings.Join(installed, ", "))
}

// InstalledModels is replaced in tests: what Ollama holds is not a fact about
// the machine a test runs on, and every plugin's tests need it held still.
var InstalledModels = ollamaModels

// pickModel finds a model when nobody configured one. A model already loaded is
// free to use; one that is merely installed costs a load but no decision.
func pickModel(host string, port int) (string, []string) {
	loaded, installed := InstalledModels(host, port)
	if len(loaded) > 0 {
		return loaded[0], installed
	}
	if len(installed) == 1 {
		return installed[0], installed
	}
	return "", installed
}

// ollamaModels asks what is loaded and what is installed, and says nothing at
// all when Ollama is not there.
func ollamaModels(host string, port int) (loaded, installed []string) {
	base := fmt.Sprintf("http://%s:%d", host, port)
	return modelNames(base + "/api/ps"), modelNames(base + "/api/tags")
}

func modelNames(url string) []string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil
	}
	var decoded struct {
		Models []struct {
			Name string `json:"name"`
		} `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil
	}
	names := make([]string, 0, len(decoded.Models))
	for _, m := range decoded.Models {
		if name := strings.TrimSpace(m.Name); name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	return names
}
