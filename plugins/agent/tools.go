package agent

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"gaia/plugins/ask"
	"gaia/plugins/shared/execpolicy"
)

// Kind is what a permission is granted over, so a new tool needs no new rule.
type Kind int

const (
	// Reads change nothing, and are always allowed.
	Reads Kind = iota
	// Writes change files inside the workspace.
	Writes
	// Runs execute a program, through the command policy.
	Runs
)

// Tool is one capability offered to the model.
type Tool struct {
	Name        string
	Description string
	Kind        Kind
	// Parameters is the JSON Schema the model is given.
	Parameters map[string]any
	// Run returns what the model sees; an error is a result too.
	Run func(ctx context.Context, call ask.ToolCall) (string, error)
}

// Permissions is what an agent may do, decided before the model says anything.
type Permissions struct {
	// AllowWrites lets the agent change files; off still allows a full review.
	AllowWrites bool
	// Policy decides which commands may run.
	Policy execpolicy.Policy
	// ConfirmRun asks the person. Nil means nobody can be asked, so it does not run.
	ConfirmRun func(message string) (bool, error)
	// MaxFileBytes caps one read, so a vendored file cannot fill the context window.
	MaxFileBytes int
	// CommandTimeout bounds a single command.
	CommandTimeout time.Duration
}

// DefaultPermissions is read-only: useful without anybody deciding anything.
func DefaultPermissions() Permissions {
	return Permissions{
		AllowWrites: false,
		Policy:      execpolicy.NewDefaultPolicy(),
		// Measured: 64KB is ~16000 tokens, more than the whole window. A file
		// that size drowns a small model, which then repeats itself until
		// ollama stops it.
		MaxFileBytes:   8 * 1024,
		CommandTimeout: 2 * time.Minute,
	}
}

// Toolset is the tools an agent may use, bound to a workspace and permissions.
type Toolset struct {
	ws          *Workspace
	perms       Permissions
	tools       map[string]Tool
	order       []string
	writtenFile map[string]bool
}

// NewToolset derives tools from permissions, so none is offered then refused.
func NewToolset(ws *Workspace, perms Permissions) *Toolset {
	perms.Policy.Workspace = ws.Root()
	ts := &Toolset{ws: ws, perms: perms, tools: map[string]Tool{}, writtenFile: map[string]bool{}}

	ts.add(Tool{
		Name:        "list_files",
		Description: "List files and directories at a path inside the project. Use it to find your way around before reading anything.",
		Kind:        Reads,
		Parameters: object(map[string]any{
			"path": property("string", "Directory relative to the project root. Defaults to the root."),
		}),
		Run: ts.listFiles,
	})

	ts.add(Tool{
		Name:        "read_file",
		Description: "Read a file from the project. Read before you change anything.",
		Kind:        Reads,
		Parameters: object(map[string]any{
			"path":  property("string", "File relative to the project root."),
			"start": property("integer", "First line to read, 1-based. Optional."),
			"end":   property("integer", "Last line to read, inclusive. Optional."),
		}, "path"),
		Run: ts.readFile,
	})

	ts.add(Tool{
		Name: "search_text",
		// Spelled out: a vaguer description had models searching for whole sentences.
		Description: "Search the project for an exact piece of text, like grep. " +
			"Give it an identifier or a literal fragment of code — a function name, a type, " +
			"a string in the source — never a description of what you are looking for. " +
			"Use it to locate a symbol before reading files one by one.",
		Kind: Reads,
		Parameters: object(map[string]any{
			"pattern": property("string", "The exact text to find, e.g. 'func NewServer' or 'ErrNotFound'."),
			"path":    property("string", "Directory to search under. Defaults to the root."),
		}, "pattern"),
		Run: ts.searchText,
	})

	if perms.AllowWrites {
		ts.add(Tool{
			Name: "edit_file",
			Description: "Change part of a file: replace an exact piece of text with another. " +
				"This is how you change an existing file — quote just the lines you are changing, " +
				"not the whole file. The old text must appear exactly once.",
			Kind: Writes,
			Parameters: object(map[string]any{
				"path": property("string", "File relative to the project root."),
				"old":  property("string", "The exact text to replace, copied from the file."),
				"new":  property("string", "What to put in its place."),
			}, "path", "old", "new"),
			Run: ts.editFile,
		})
		ts.add(Tool{
			Name: "write_file",
			Description: "Create a new file, or replace a whole one. The content must be the " +
				"complete file. To change part of a file that already exists, use edit_file instead.",
			Kind: Writes,
			Parameters: object(map[string]any{
				"path":    property("string", "File relative to the project root."),
				"content": property("string", "The complete new contents of the file."),
			}, "path", "content"),
			Run: ts.writeFile,
		})
	}

	ts.add(Tool{
		Name: "read_symbol",
		Description: "Read one function, type or variable from a Go file, by name, with the " +
			"comment above it. Prefer this to read_file: a whole file costs your context " +
			"every turn it stays there. Methods are named Type.Method.",
		Kind: Reads,
		Parameters: object(map[string]any{
			"path": property("string", "File relative to the project root."),
			"name": property("string", "What to read, e.g. BuildMemoryContext or Toolset.Call."),
		}, "path", "name"),
		Run: ts.readSymbol,
	})

	ts.add(Tool{
		Name: "run_command",
		Description: "Run a command in the project. There is no shell: redirects, semicolons and " +
			"$(…) are refused. Pipes work — each side is checked separately — so `ls -1 *.go | wc -l` " +
			"is fine. Use it for builds, tests and git.",
		Kind: Runs,
		Parameters: object(map[string]any{
			"command": property("string", "The command line, e.g. 'go test ./...'."),
		}, "command"),
		Run: ts.runCommand,
	})

	return ts
}

// NewCommandToolset is what investigate uses: a machine answers to commands, not reads.
func NewCommandToolset(ws *Workspace, perms Permissions) *Toolset {
	perms.AllowWrites = false
	// Not confined: investigating a machine means reading /var and /etc, which
	// is the difference between this and an agent on a project.
	ts := &Toolset{ws: ws, perms: perms, tools: map[string]Tool{}, writtenFile: map[string]bool{}}
	ts.add(Tool{
		Name: "run_command",
		Description: "Run a command on this machine. There is no shell: redirects, semicolons " +
			"and $(…) are refused. Pipes work — each side is checked separately — so " +
			"`df -h | grep /dev` is fine.",
		Kind: Runs,
		Parameters: object(map[string]any{
			"command": property("string", "The command line, e.g. 'df -h' or 'ls -la /var/log'."),
		}, "command"),
		Run: ts.runCommand,
	})
	return ts
}

func (ts *Toolset) add(tool Tool) {
	ts.tools[tool.Name] = tool
	ts.order = append(ts.order, tool.Name)
}

// Specs renders the tools in a stable order: two runs differ by model, not by map.
func (ts *Toolset) Specs() []ask.ToolSpec {
	specs := make([]ask.ToolSpec, 0, len(ts.order))
	for _, name := range ts.order {
		tool := ts.tools[name]
		specs = append(specs, ask.ToolSpec{
			Name:        tool.Name,
			Description: tool.Description,
			Parameters:  tool.Parameters,
		})
	}
	return specs
}

// Names lists the tools available, for a transcript or a prompt.
func (ts *Toolset) Names() []string {
	return append([]string(nil), ts.order...)
}

// FilesWritten is what a person checks at the end: forty files for a one-line fix.
func (ts *Toolset) FilesWritten() []string {
	out := make([]string, 0, len(ts.writtenFile))
	for path := range ts.writtenFile {
		out = append(out, path)
	}
	sort.Strings(out)
	return out
}

// Call returns a failure as a result: a wrong path is to correct, not to stop on.
func (ts *Toolset) Call(ctx context.Context, call ask.ToolCall) string {
	tool, ok := ts.tools[call.Name]
	if !ok {
		return fmt.Sprintf("error: there is no tool called %q. Available: %s",
			call.Name, strings.Join(ts.Names(), ", "))
	}
	out, err := tool.Run(ctx, call)
	if err != nil {
		if better := ts.toolThatTakes(call); better != "" {
			return fmt.Sprintf("error: %v. Those arguments belong to %s — call that instead.",
				err, better)
		}
		return "error: " + err.Error()
	}
	if strings.TrimSpace(out) == "" {
		// An empty result otherwise reads as a broken tool.
		return "(no output)"
	}
	return out
}

// --- the tools themselves -------------------------------------------------

func (ts *Toolset) listFiles(_ context.Context, call ask.ToolCall) (string, error) {
	path, err := ts.ws.Resolve(call.ArgStringOr("path", "."))
	if err != nil {
		return "", err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	for _, entry := range entries {
		name := entry.Name()
		// .git burns steps on object files; node_modules fills any context window.
		if name == ".git" || name == "node_modules" || name == "vendor" {
			continue
		}
		if entry.IsDir() {
			b.WriteString(name + "/\n")
			continue
		}
		b.WriteString(name + "\n")
	}
	return strings.TrimSpace(b.String()), nil
}

func (ts *Toolset) readFile(_ context.Context, call ask.ToolCall) (string, error) {
	rel, err := call.ArgString("path")
	if err != nil {
		return "", err
	}
	path, err := ts.ws.Resolve(rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- Resolve confines this to the workspace
	if err != nil {
		return "", err
	}

	if len(data) == 0 {
		// "1\t" would read as a tool that half worked.
		return fmt.Sprintf("%s is empty", ts.ws.Relative(path)), nil
	}

	lines := strings.Split(string(data), "\n")
	start := clampLine(call, "start", 1, len(lines))
	end := clampLine(call, "end", len(lines), len(lines))
	if end < start {
		end = start
	}

	var b strings.Builder
	total := 0
	for i := start - 1; i < end && i < len(lines); i++ {
		// Numbered: the next thing a model does is talk about a line.
		line := fmt.Sprintf("%d\t%s\n", i+1, lines[i])
		if ts.perms.MaxFileBytes > 0 && total+len(line) > ts.perms.MaxFileBytes {
			fmt.Fprintf(&b, "... (truncated at %d bytes; read a narrower range with start and end)\n",
				ts.perms.MaxFileBytes)
			break
		}
		total += len(line)
		b.WriteString(line)
	}
	return strings.TrimRight(b.String(), "\n"), nil
}

// clampLine reads an optional line number, keeping it inside the file.
func clampLine(call ask.ToolCall, name string, fallback, highest int) int {
	raw := call.ArgStringOr(name, "")
	if raw == "" {
		return fallback
	}
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return fallback
	}
	if n > highest {
		return highest
	}
	return n
}

func (ts *Toolset) searchText(ctx context.Context, call ask.ToolCall) (string, error) {
	pattern, err := call.ArgString("pattern")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(pattern) == "" {
		return "", fmt.Errorf("search_text: the pattern is empty, which would match every line in the project")
	}
	root, err := ts.ws.Resolve(call.ArgStringOr("path", "."))
	if err != nil {
		return "", err
	}

	files, err := ts.searchableFiles(ctx, root)
	if err != nil {
		return "", err
	}

	const maxHits = 100
	var b strings.Builder
	hits := 0
	for _, path := range files {
		data, readErr := os.ReadFile(path) // #nosec G304 -- inside the workspace by construction
		if readErr != nil || isBinary(data) {
			continue
		}
		for i, line := range strings.Split(string(data), "\n") {
			if !strings.Contains(line, pattern) {
				continue
			}
			fmt.Fprintf(&b, "%s:%d: %s\n", ts.ws.Relative(path), i+1, strings.TrimSpace(line))
			hits++
			if hits >= maxHits {
				fmt.Fprintf(&b, "... (stopped at %d matches; search something narrower)\n", maxHits)
				return strings.TrimSpace(b.String()), nil
			}
		}
	}
	if hits == 0 {
		// "nothing" is an answer to act on, not a tool that seemed not to work.
		return fmt.Sprintf("no file under %s contains %q", ts.ws.Relative(root), pattern), nil
	}
	return strings.TrimSpace(b.String()), nil
}

// searchableFiles is what git tracks, else a walk: generated files buried the code.
func (ts *Toolset) searchableFiles(ctx context.Context, root string) ([]string, error) {
	if tracked, ok := ts.trackedFiles(ctx, root); ok {
		return tracked, nil
	}

	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return nil //nolint:nilerr // an unreadable subtree is not a failed search
		}
		if entry.IsDir() {
			switch entry.Name() {
			case ".git", "node_modules", "vendor", "bin", "dist", "build":
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, path)
		return nil
	})
	return files, err
}

// trackedFiles asks git; the bool says whether to fall back to a walk.
func (ts *Toolset) trackedFiles(ctx context.Context, root string) ([]string, bool) {
	result, err := execpolicy.RunIn(ctx, root, []string{"git", "ls-files", "-z"}, 30*time.Second)
	if err != nil || result.ExitCode != 0 {
		return nil, false
	}
	var files []string
	for _, name := range strings.Split(result.Stdout, "\x00") {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		files = append(files, filepath.Join(root, name))
	}
	if len(files) == 0 {
		return nil, false
	}
	sort.Strings(files)
	return files, true
}

// isBinary: a NUL byte in the first few hundred is the usual tell.
func isBinary(data []byte) bool {
	scan := min(len(data), 512)
	for i := 0; i < scan; i++ {
		if data[i] == 0 {
			return true
		}
	}
	return false
}

func (ts *Toolset) writeFile(_ context.Context, call ask.ToolCall) (string, error) {
	if !ts.perms.AllowWrites {
		return "", fmt.Errorf("this agent may not change files")
	}
	rel, err := call.ArgString("path")
	if err != nil {
		return "", err
	}
	content, err := call.ArgString("content")
	if err != nil {
		return "", err
	}
	path, err := ts.ws.Resolve(rel)
	if err != nil {
		return "", err
	}

	if err := ts.refuseTruncation(path, content); err != nil {
		return "", err
	}
	if err := checkSyntax(ts.ws.Relative(path), content); err != nil {
		return "", fmt.Errorf("write_file: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		return "", err
	}

	ts.writtenFile[ts.ws.Relative(path)] = true
	return fmt.Sprintf("wrote %s (%d bytes)", ts.ws.Relative(path), len(content)), nil
}

// minimumKeptFraction: below this, a whole-file write is a fragment, not a file.
const minimumKeptFraction = 0.5

// refuseTruncation stops a model replacing a file it could not reproduce. Asked
// to change five lines of five hundred, a small model writes the five.
func (ts *Toolset) refuseTruncation(path, content string) error {
	existing, err := os.ReadFile(path) // #nosec G304 -- resolved inside the workspace
	if err != nil {
		return nil // a new file has nothing to lose
	}
	if len(existing) < 400 {
		return nil // too small for the ratio to mean anything
	}
	if float64(len(content)) >= float64(len(existing))*minimumKeptFraction {
		return nil
	}
	return fmt.Errorf("refusing to write %s: it is %d bytes and you supplied %d, "+
		"which would delete most of it. If you meant to change part of it, use edit_file "+
		"and quote only the lines you are changing",
		ts.ws.Relative(path), len(existing), len(content))
}

// editFile replaces one exact piece of text, which is how a file is changed
// without reproducing it.
func (ts *Toolset) editFile(_ context.Context, call ask.ToolCall) (string, error) {
	if !ts.perms.AllowWrites {
		return "", fmt.Errorf("this agent may not change files")
	}
	rel, err := call.ArgString("path")
	if err != nil {
		return "", err
	}
	oldText, err := call.ArgString("old")
	if err != nil {
		return "", err
	}
	newText, err := call.ArgString("new")
	if err != nil {
		return "", err
	}
	if oldText == "" {
		return "", fmt.Errorf("edit_file: the text to replace is empty; use write_file to create a file")
	}
	// read_file numbers its lines, so that is how a model quotes them back.
	oldText = stripLineNumbers(oldText)
	newText = stripLineNumbers(newText)
	path, err := ts.ws.Resolve(rel)
	if err != nil {
		return "", err
	}
	existing, err := os.ReadFile(path) // #nosec G304 -- resolved inside the workspace
	if err != nil {
		return "", err
	}

	start, end, found, unique, unescaped := findBlockTolerant(string(existing), oldText)
	if unescaped {
		newText = unescapeSequences(newText)
	}
	if !found {
		return "", fmt.Errorf("edit_file: that text is not in %s. %s", ts.ws.Relative(path),
			nearestLines(string(existing), oldText))
	}
	if !unique {
		return "", fmt.Errorf("edit_file: that text appears more than once in %s, so which one "+
			"is meant is unclear. Quote more of the surrounding lines", ts.ws.Relative(path))
	}

	replaced := string(existing)[start:end]
	updated := string(existing)[:start] +
		reindent(newText, firstNonEmptyLine(replaced)) +
		string(existing)[end:]
	if err := checkSyntax(ts.ws.Relative(path), updated); err != nil {
		return "", fmt.Errorf("edit_file: %w", err)
	}
	if err := os.WriteFile(path, []byte(updated), 0o600); err != nil {
		return "", err
	}
	ts.writtenFile[ts.ws.Relative(path)] = true
	return fmt.Sprintf("edited %s (%d bytes, was %d)",
		ts.ws.Relative(path), len(updated), len(existing)), nil
}

func (ts *Toolset) readSymbol(_ context.Context, call ask.ToolCall) (string, error) {
	rel, err := call.ArgString("path")
	if err != nil {
		return "", err
	}
	name, err := call.ArgString("name")
	if err != nil {
		return "", err
	}
	path, err := ts.ws.Resolve(rel)
	if err != nil {
		return "", err
	}
	data, err := os.ReadFile(path) // #nosec G304 -- resolved inside the workspace
	if err != nil {
		return "", err
	}
	source, _, err := findSymbol(string(data), name)
	if err != nil {
		return "", fmt.Errorf("read_symbol %s: %w", ts.ws.Relative(path), err)
	}
	return source, nil
}

func (ts *Toolset) runCommand(ctx context.Context, call ask.ToolCall) (string, error) {
	line, err := call.ArgString("command")
	if err != nil {
		return "", err
	}

	decision := ts.perms.Policy.Decide(line)
	switch decision.Verdict {
	case execpolicy.Refuse:
		return "", fmt.Errorf("refused: %s", decision.Reason)
	case execpolicy.Confirm:
		if ts.perms.ConfirmRun == nil {
			// Fail closed: silence is not consent.
			return "", fmt.Errorf("%s needs confirmation and there is nobody to ask", decision.Key)
		}
		confirmed, confirmErr := ts.perms.ConfirmRun("Run command: " + line)
		if confirmErr != nil {
			return "", fmt.Errorf("confirmation failed: %w", confirmErr)
		}
		if !confirmed {
			return "", fmt.Errorf("declined: %s", line)
		}
	}

	result, err := execpolicy.RunPipelineIn(ctx, ts.ws.Root(), decision.Stages, ts.perms.CommandTimeout)
	if err != nil {
		return "", err
	}

	var b strings.Builder
	if result.Stdout != "" {
		b.WriteString(result.Stdout + "\n")
	}
	if result.Stderr != "" {
		b.WriteString("stderr:\n" + result.Stderr + "\n")
	}
	// The code is part of the answer: a passing and a failing run look alike.
	fmt.Fprintf(&b, "exit code: %d", result.ExitCode)
	return b.String(), nil
}

// --- schema helpers -------------------------------------------------------

func object(properties map[string]any, required ...string) map[string]any {
	schema := map[string]any{"type": "object", "properties": properties}
	if len(required) > 0 {
		schema["required"] = required
	}
	return schema
}

func property(typ, description string) map[string]any {
	return map[string]any{"type": typ, "description": description}
}

// lineNumberPrefix is what read_file puts in front of every line.
var lineNumberPrefix = regexp.MustCompile(`(?m)^\d+\t`)

// stripLineNumbers lets a model quote straight back from what read_file showed
// it, which is the only copy of the file it has.
func stripLineNumbers(text string) string {
	if !lineNumberPrefix.MatchString(text) {
		return text
	}
	return lineNumberPrefix.ReplaceAllString(text, "")
}

// nearestLines shows what is actually there, so a failed match is one step from
// a good one rather than another guess.
func nearestLines(content, wanted string) string {
	first := strings.TrimSpace(firstNonEmptyLine(wanted))
	if first == "" {
		return "Read it again and copy the lines exactly, whitespace included."
	}
	for _, probe := range []string{first, trimToWords(first, 4), trimToWords(first, 2)} {
		if probe == "" {
			continue
		}
		if i := strings.Index(content, probe); i >= 0 {
			return fmt.Sprintf("The closest thing in the file is:\n\n%s\n\nCopy it exactly, "+
				"whitespace included.", excerptAt(content, i))
		}
	}
	return "Read it again and copy the lines exactly, whitespace included."
}

func firstNonEmptyLine(text string) string {
	for _, line := range strings.Split(text, "\n") {
		if strings.TrimSpace(line) != "" {
			return line
		}
	}
	return ""
}

func trimToWords(line string, n int) string {
	fields := strings.Fields(line)
	if len(fields) < n {
		return ""
	}
	return strings.Join(fields[:n], " ")
}

// excerptAt shows the matched line and the two after it, which is usually the
// whole of what was being edited.
func excerptAt(content string, index int) string {
	start := strings.LastIndexByte(content[:index], '\n') + 1
	lines := strings.SplitN(content[start:], "\n", 4)
	if len(lines) > 3 {
		lines = lines[:3]
	}
	return strings.Join(lines, "\n")
}

// toolThatTakes names the tool whose required arguments the call actually
// carries. A model that reaches for the wrong one repeats the mistake until
// something tells it which one it wanted.
func (ts *Toolset) toolThatTakes(call ask.ToolCall) string {
	if len(call.Arguments) == 0 {
		return ""
	}
	// Only when the call is missing what it needs: a tool used correctly may
	// still fail, and that failure is not about which tool was chosen.
	if hasAll(call.Arguments, ts.requiredArgs(call.Name)) {
		return ""
	}
	for _, name := range ts.Names() {
		if name == call.Name {
			continue
		}
		if required := ts.requiredArgs(name); len(required) > 0 && hasAll(call.Arguments, required) {
			return name
		}
	}
	return ""
}

func (ts *Toolset) requiredArgs(name string) []string {
	tool, ok := ts.tools[name]
	if !ok {
		return nil
	}
	raw, ok := tool.Parameters["required"].([]string)
	if !ok {
		return nil
	}
	return raw
}

func hasAll(args map[string]any, required []string) bool {
	for _, key := range required {
		if _, ok := args[key]; !ok {
			return false
		}
	}
	return true
}
