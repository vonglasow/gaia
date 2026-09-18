package agent

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"gaia/plugins/ask"
)

// StopReason says why a run ended: a truncated run reads like a complete one.
type StopReason string

const (
	// StopAnswered is the model saying it is done, with no tool call attached.
	StopAnswered StopReason = "answered"
	// StopMaxSteps is the ceiling on turns.
	StopMaxSteps StopReason = "reached the step limit"
	// StopRepeating is the same call twice running: a model stuck, not working.
	StopRepeating StopReason = "repeating itself"
	// StopCancelled is ctrl-c, or a timeout on the whole run.
	StopCancelled StopReason = "cancelled"
	// StopProviderFailed is the model becoming unreachable partway through.
	StopProviderFailed StopReason = "the model could not be reached"
	// StopToolsIgnored is a model narrating calls it never made, usually one too small.
	StopToolsIgnored StopReason = "the model wrote tool calls as text instead of making them"
)

// Step is one turn, kept so a run can be read back rather than inferred.
type Step struct {
	Number    int
	ToolCalls []ask.ToolCall
	Results   []string
	Text      string
	// Recovered says the call was written as text and read back out of it.
	Recovered bool
}

// Result is everything a run produced.
type Result struct {
	Answer       string
	Steps        []Step
	StopReason   StopReason
	FilesWritten []string
}

// Options configures one run.
type Options struct {
	// Task is what the person asked for.
	Task string
	// SystemPrompt is the role the model works under.
	SystemPrompt string
	// MaxSteps bounds the turns. Zero means the default.
	MaxSteps int
	// Send asks the model; injected so the loop can be driven without one.
	Send func(context.Context, ask.AskRequest) (ask.AskResponse, error)
	// Tools is what the model may use.
	Tools *Toolset
	// Observe reports each turn as it happens: tool, arguments, result.
	Observe func(Step)
	// TokenBudget is what the conversation may occupy, the rules and the room
	// to answer already taken out. Zero leaves it untrimmed.
	TokenBudget int
}

// DefaultMaxSteps: enough for a real task, few enough that a stuck run costs a minute.
const DefaultMaxSteps = 20

// ErrNoTask is a mistake at the command line, not something to ask a model about.
var ErrNoTask = errors.New("there is no task to work on")

// Run drives the loop until something stops it.
func Run(ctx context.Context, opts Options) (Result, error) {
	task := strings.TrimSpace(opts.Task)
	if task == "" {
		return Result{}, ErrNoTask
	}
	if opts.Send == nil {
		return Result{}, errors.New("no model to ask")
	}
	if opts.Tools == nil {
		return Result{}, errors.New("no tools to work with")
	}
	maxSteps := opts.MaxSteps
	if maxSteps <= 0 {
		maxSteps = DefaultMaxSteps
	}

	messages := []ask.ChatMessage{{Role: "user", Content: task}}
	result := Result{StopReason: StopMaxSteps}
	lastCallSignature := ""

	for turn := 1; turn <= maxSteps; turn++ {
		if err := ctx.Err(); err != nil {
			result.StopReason = StopCancelled
			result.FilesWritten = opts.Tools.FilesWritten()
			return result, nil
		}

		messages = trimConversation(messages, opts.TokenBudget)
		resp, err := opts.Send(ctx, ask.AskRequest{
			SystemPrompt: opts.SystemPrompt,
			Messages:     messages,
			Tools:        opts.Tools.Specs(),
		})
		if err != nil {
			result.StopReason = StopProviderFailed
			result.FilesWritten = opts.Tools.FilesWritten()
			return result, err
		}

		step := Step{Number: turn, Text: strings.TrimSpace(resp.Text), ToolCalls: resp.ToolCalls}

		// A model that wrote the call instead of making it is one step from the
		// answer, not finished. Recover it rather than stopping.
		if len(resp.ToolCalls) == 0 {
			if call, ok := ask.ToolCallInText(step.Text, opts.Tools.Names()); ok {
				resp.ToolCalls = []ask.ToolCall{call}
				step.ToolCalls = resp.ToolCalls
				step.Recovered = true
			}
		}

		// No calls ends the task, unless the answer merely describes the calls.
		if len(resp.ToolCalls) == 0 {
			result.Answer = step.Text
			result.StopReason = StopAnswered
			if describesAToolCall(step.Text, opts.Tools.Names()) {
				result.StopReason = StopToolsIgnored
			}
			result.Steps = append(result.Steps, step)
			observe(opts, step)
			result.FilesWritten = opts.Tools.FilesWritten()
			return result, nil
		}

		// One repeat is legitimate after a change; two identical turns is a loop.
		signature := callSignature(resp.ToolCalls)
		if signature == lastCallSignature {
			result.StopReason = StopRepeating
			result.Steps = append(result.Steps, step)
			observe(opts, step)
			result.Answer = step.Text
			result.FilesWritten = opts.Tools.FilesWritten()
			return result, nil
		}
		lastCallSignature = signature

		messages = append(messages, ask.ChatMessage{
			Role:      "assistant",
			Content:   step.Text,
			ToolCalls: resp.ToolCalls,
		})

		for _, call := range resp.ToolCalls {
			out := opts.Tools.Call(ctx, call)
			step.Results = append(step.Results, out)
			messages = append(messages, ask.ChatMessage{
				Role:     "tool",
				ToolName: call.Name,
				Content:  out,
			})
		}
		messages = append(messages, ask.ChatMessage{Role: "user", Content: remind(task)})

		result.Steps = append(result.Steps, step)
		observe(opts, step)
	}

	result.FilesWritten = opts.Tools.FilesWritten()
	if len(result.Steps) > 0 {
		result.Answer = result.Steps[len(result.Steps)-1].Text
	}
	return result, nil
}

// describesAToolCall looks for a tool name next to a bracket, so prose does not match.
func describesAToolCall(text string, toolNames []string) bool {
	if strings.TrimSpace(text) == "" {
		return false
	}
	for _, name := range toolNames {
		for _, form := range []string{name + "(", name + `"`, name + "\":"} {
			if strings.Contains(text, form) {
				return true
			}
		}
	}
	return false
}

func observe(opts Options, step Step) {
	if opts.Observe != nil {
		opts.Observe(step)
	}
}

// callSignature compares turns. Arguments count: two files is progress, one twice is not.
func callSignature(calls []ask.ToolCall) string {
	var b strings.Builder
	for _, call := range calls {
		b.WriteString(call.Name)
		b.WriteString("(")
		// Sorted, so map order does not make two identical calls look different.
		keys := sortedKeys(call.Arguments)
		for _, key := range keys {
			value, _ := call.ArgString(key)
			fmt.Fprintf(&b, "%s=%s,", key, value)
		}
		b.WriteString(")")
	}
	return b.String()
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	// Sorted so a signature is stable across turns.
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}
	return keys
}

// remind restates the task after tool output. A file read puts hundreds of lines
// between a model and what it was asked for, and a small one answers about what
// it just read instead of doing the job.
func remind(task string) string {
	return "Your task, which the output above is only material for: " + task +
		"\n\nDo it. Do not describe the code, summarise it, or explain what the " +
		"functions do. Call a tool to make the change, or answer only when it is done."
}
