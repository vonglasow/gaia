// Package models reports which models are resident in Ollama, and unloads them.
package models

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"gaia/kernel"
	"gaia/plugins/ask"
	"gaia/plugins/shared"
)

// requestTimeout bounds a call to the local daemon, which answers in milliseconds.
const requestTimeout = 15 * time.Second

type ModelsPlugin struct {
	kernel.BasePlugin
}

func NewModelsPlugin() *ModelsPlugin { return &ModelsPlugin{} }

func (p *ModelsPlugin) ID() string           { return "models" }
func (p *ModelsPlugin) DefaultEnabled() bool { return true }

func (p *ModelsPlugin) ConfigSchema() []string {
	return []string{"models.host", "models.port"}
}

func (p *ModelsPlugin) Register(_ *kernel.Kernel) ([]*cobra.Command, error) {
	root := &cobra.Command{
		Use:   "models",
		Short: "See which models are loaded, and unload them",
		Long: "A model stays resident after it answers — five minutes by default, which is\n" +
			"18 GB for a 30B. This is how to see that and take it back.",
		RunE: p.runList,
	}

	root.AddCommand(&cobra.Command{
		Use:   "unload [model]",
		Short: "Unload a model now, or every model with no argument",
		Args:  cobra.MaximumNArgs(1),
		RunE:  p.runUnload,
	})

	return []*cobra.Command{root}, nil
}

// loaded is one resident model as /api/ps reports it.
type loaded struct {
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	SizeVRAM  int64     `json:"size_vram"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (p *ModelsPlugin) runList(cmd *cobra.Command, _ []string) error {
	models, err := listLoaded(cmd.Context(), baseURL())
	if err != nil {
		return shared.Fail(cmd.ErrOrStderr(), err.Error())
	}
	return shared.PrintBox(cmd.OutOrStdout(), "Models", renderLoaded(models, time.Now()))
}

func (p *ModelsPlugin) runUnload(cmd *cobra.Command, args []string) error {
	base := baseURL()

	names := args
	if len(names) == 0 {
		models, err := listLoaded(cmd.Context(), base)
		if err != nil {
			return shared.Fail(cmd.ErrOrStderr(), err.Error())
		}
		for _, m := range models {
			names = append(names, m.Name)
		}
	}
	if len(names) == 0 {
		return shared.PrintBox(cmd.OutOrStdout(), "Models", "Nothing is loaded.")
	}

	for _, name := range names {
		if err := unload(cmd.Context(), base, name); err != nil {
			return shared.Failf(cmd.ErrOrStderr(), "Unloading %s: %v", name, err)
		}
	}
	return shared.PrintBox(cmd.OutOrStdout(), "Models", "Unloaded "+strings.Join(names, ", "))
}

// renderLoaded describes what is resident, and what it costs.
func renderLoaded(models []loaded, now time.Time) string {
	if len(models) == 0 {
		return "Nothing is loaded."
	}
	sort.Slice(models, func(i, j int) bool { return models[i].Name < models[j].Name })

	var b strings.Builder
	var total int64
	for _, m := range models {
		total += m.Size
		fmt.Fprintf(&b, "%s\t%s\t%s\n", m.Name, humanBytes(m.Size), until(m.ExpiresAt, now))
	}
	fmt.Fprintf(&b, "\n%s resident", humanBytes(total))
	return b.String()
}

// until says how long a model will stay, which is what makes the cost actionable.
func until(expires, now time.Time) string {
	if expires.IsZero() {
		return "no expiry"
	}
	left := expires.Sub(now).Round(time.Second)
	if left <= 0 {
		return "expiring"
	}
	return "for " + left.String()
}

func humanBytes(n int64) string {
	const unit = 1024
	if n < unit {
		return fmt.Sprintf("%d B", n)
	}
	div, exp := int64(unit), 0
	for size := n / unit; size >= unit; size /= unit {
		div *= unit
		exp++
	}
	return fmt.Sprintf("%.1f %cB", float64(n)/float64(div), "KMGTPE"[exp])
}

// listLoaded asks Ollama what it is holding.
func listLoaded(ctx context.Context, base string) ([]loaded, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/ps", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("asking %s what is loaded: %w", base, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return nil, fmt.Errorf("ollama: status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var decoded struct {
		Models []loaded `json:"models"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("reading what is loaded: %w", err)
	}
	return decoded.Models, nil
}

// unload asks for the model with keep_alive 0, which is how Ollama is told to drop it.
func unload(ctx context.Context, base, name string) error {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	body, err := json.Marshal(map[string]any{"model": name, "keep_alive": 0})
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/generate", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		return fmt.Errorf("status=%d body=%s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)
	return nil
}

// baseURL points at the same Ollama the rest of gaia talks to.
func baseURL() string {
	host, port := ask.Address("models")
	return fmt.Sprintf("http://%s:%d", host, port)
}
