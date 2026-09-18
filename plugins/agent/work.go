package agent

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"

	"gaia/config"
	"gaia/plugins/ask"
	"gaia/plugins/mempalace"
	"gaia/plugins/shared"
	"gaia/plugins/shared/execpolicy"
)

// Work is one run of a model with tools, from the task to the printed answer.
//
// What differs between `gaia agent` and `gaia investigate` is the workspace,
// the toolset and the prompt; everything after that is the same run.
type Work struct {
	// Plugin namespaces the config keys and labels what is remembered.
	Plugin string
	// Title heads the box the answer is printed in.
	Title string
	Task  string

	Tools    *Toolset
	Prompt   string
	Request  ask.AskRequest
	Provider ask.Provider
	MaxSteps int

	// Quiet prints the answer and nothing else.
	Quiet bool
	// Unload drops the model from memory when the run ends.
	Unload bool
	// Writing says the run was allowed to change files, so a run that changed
	// none is worth saying out loud rather than reading as success.
	Writing bool
	// Render turns one step into a transcript line, or "" to print nothing.
	Render func(Step) string
}

// PermissionsFor builds what a model may do, from the plugin's own config keys.
func PermissionsFor(cmd *cobra.Command, plugin string) Permissions {
	perms := DefaultPermissions()
	perms.Policy = execpolicy.NewDefaultPolicy().WithExtra(
		config.StringList(plugin+".allowlist"),
		config.StringList(plugin+".denylist"),
	)
	if yes, _ := cmd.Flags().GetBool("yes"); yes {
		perms.Policy.AllowAll = true
	}
	perms.ConfirmRun = shared.ConfirmWith(cmd.InOrStdin(), cmd.OutOrStdout())
	if seconds := viper.GetInt(plugin + ".command_timeout_seconds"); seconds > 0 {
		perms.CommandTimeout = time.Duration(seconds) * time.Second
	}
	if maxBytes := viper.GetInt(plugin + ".max_file_bytes"); maxBytes > 0 {
		perms.MaxFileBytes = maxBytes
	}
	return perms
}

// SendVia fills in the endpoint and sanitises, so a run never leaves more of
// the machine than `gaia ask` would.
func SendVia(provider ask.Provider, req ask.AskRequest, progressOut interface{ Write([]byte) (int, error) }) func(context.Context, ask.AskRequest) (ask.AskResponse, error) {
	return func(ctx context.Context, r ask.AskRequest) (ask.AskResponse, error) {
		r.Provider = req.Provider
		r.Host = req.Host
		r.Port = req.Port
		r.Model = req.Model
		r.Timeout = req.Timeout
		r.ContextWindow = req.ContextWindow
		r.ProgressOut = progressOut
		if progressOut != nil {
			r = ask.ApplySanitize(progressOut, r)
		}
		resp, err := provider.Send(ctx, r)
		return resp, ask.DescribeSendError(err, r)
	}
}

// Do runs the loop, remembers the answer and prints it.
func (w Work) Do(cmd *cobra.Command) error {
	maxSteps := w.MaxSteps
	if maxSteps <= 0 {
		maxSteps = viper.GetInt(w.Plugin + ".max_steps")
	}

	// Fixed for the run: Ollama reloads the model whenever num_ctx changes.
	req := w.Request
	req.ContextWindow = ask.WindowForRun(ask.AskRequest{
		SystemPrompt: w.Prompt, Message: w.Task, Tools: w.Tools.Specs(),
	})

	result, err := Run(cmd.Context(), Options{
		Task:         w.Task,
		SystemPrompt: w.Prompt,
		MaxSteps:     maxSteps,
		Tools:        w.Tools,
		TokenBudget:  ask.ConversationBudget(req.ContextWindow, w.Prompt, w.Tools.Specs()),
		Send:         SendVia(w.Provider, req, cmd.ErrOrStderr()),
		Observe: func(step Step) {
			if w.Quiet || w.Render == nil {
				return
			}
			if line := w.Render(step); line != "" {
				_ = shared.PrintRaw(cmd.ErrOrStderr(), line)
			}
		},
	})
	if err != nil {
		return shared.Fail(cmd.ErrOrStderr(), err.Error())
	}

	if w.Unload {
		w.unload(cmd)
	}

	answer := strings.TrimSpace(result.Answer)
	if answer == "" {
		answer = "(the model said nothing)"
	}

	// Remembered before printing, so a run read and forgotten is still kept.
	w.remember(cmd, answer)

	if !w.Quiet && len(result.FilesWritten) > 0 {
		_ = shared.PrintBox(cmd.OutOrStdout(), "Files changed", strings.Join(result.FilesWritten, "\n"))
	}
	if !w.Quiet && w.Writing && len(result.FilesWritten) == 0 {
		_ = shared.PrintBox(cmd.OutOrStdout(), "Files changed",
			"None. The model answered without changing anything, which for a task\n"+
				"asking for a change means it did not do it.")
	}
	if err := shared.PrintBox(cmd.OutOrStdout(), w.Title, answer); err != nil {
		return err
	}
	if result.StopReason != StopAnswered {
		// A truncated answer reads like a complete one; the exit code does not.
		return shared.Failf(cmd.ErrOrStderr(), "Stopped after %d steps: %s.", len(result.Steps), result.StopReason)
	}
	return nil
}

// unload asks for the model once more with keep_alive 0, which drops it.
func (w Work) unload(cmd *cobra.Command) {
	req := w.Request
	req.Messages, req.Message, req.Tools, req.KeepAlive = nil, "bye", nil, "0"
	if _, err := w.Provider.Send(cmd.Context(), req); err != nil {
		w.debug(cmd, "unload failed: %v", err)
	}
}

func (w Work) remember(cmd *cobra.Command, answer string) {
	if err := mempalace.PersistInvestigateResult(cmd.Context(), w.Task, answer); err != nil {
		w.debug(cmd, "mempalace persist failed: %v", err)
	}
	if err := mempalace.DiaryWriteIfEnabled(cmd.Context(), w.Task, answer); err != nil {
		w.debug(cmd, "mempalace diary write failed: %v", err)
	}
}

func (w Work) debug(cmd *cobra.Command, format string, args ...any) {
	if !viper.GetBool("debug") {
		return
	}
	_ = shared.PrintRaw(cmd.ErrOrStderr(), fmt.Sprintf("[DEBUG] "+format+"\n", args...))
}
