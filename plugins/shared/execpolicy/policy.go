package execpolicy

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"time"
)

// Verdict is what a policy decides about one command.
type Verdict int

const (
	// Refuse: never runs, whatever anyone confirms.
	Refuse Verdict = iota
	// Confirm: the default for anything unlisted, which is what fails closed.
	Confirm
	// Allow: runs without asking. Reserved for reads.
	Allow
)

func (v Verdict) String() string {
	switch v {
	case Refuse:
		return "refuse"
	case Confirm:
		return "confirm"
	case Allow:
		return "allow"
	default:
		return "unknown"
	}
}

// Decision carries a verdict and the reason a person reads before confirming.
type Decision struct {
	Verdict Verdict
	Reason  string
	Key     string
	// Argv is the first stage, for callers that only ever deal with one command.
	Argv []string
	// Stages is the whole pipeline; every stage was judged and all had to pass.
	Stages [][]string
}

// Policy holds Key-shaped entries; anything unlisted lands on Confirm.
type Policy struct {
	Allowed []string
	Refused []string
	// AllowAll is what --yes sets. It never overrides Refused.
	AllowAll bool
	// Workspace confines path arguments; empty means no confinement.
	Workspace string
}

// DefaultAllowed runs without asking: every entry reads, none writes.
var DefaultAllowed = []string{
	"ls", "cat", "head", "tail", "wc", "file", "stat", "du", "df",
	"grep", "rg", "find", "fd", "which", "type", "env", "pwd", "date",
	"echo", "sort", "uniq", "cut", "tr", "basename", "dirname", "realpath",
	"ps", "uname", "hostname", "whoami", "id",
	"git status", "git diff", "git log", "git show", "git branch",
	"git remote", "git config", "git ls-files", "git blame",
	"go version", "go env", "go list", "go vet", "go build", "go test",
	"gofmt", "golangci-lint", "make",
}

// DefaultRefused never runs. Short on purpose: a long list implies the rest is safe.
var DefaultRefused = []string{
	"sudo", "doas", "su",
	"shutdown", "reboot", "halt", "poweroff",
	"mkfs", "fdisk", "dd",
	"passwd", "chpasswd", "useradd", "userdel", "usermod",
	"curl", "wget", "nc", "ncat", "telnet", "ssh", "scp", "sftp", "rsync",
}

// NewDefaultPolicy is usable as-is: no configuration, and it still fails closed.
func NewDefaultPolicy() Policy {
	return Policy{
		Allowed: append([]string(nil), DefaultAllowed...),
		Refused: append([]string(nil), DefaultRefused...),
	}
}

// WithExtra adds to each list; a command on both is refused.
func (p Policy) WithExtra(allowed, refused []string) Policy {
	out := Policy{
		Allowed:   append(append([]string(nil), p.Allowed...), allowed...),
		Refused:   append(append([]string(nil), p.Refused...), refused...),
		AllowAll:  p.AllowAll,
		Workspace: p.Workspace,
	}
	return out
}

// Decide judges a command line. One that does not parse is refused, not confirmed.
func (p Policy) Decide(line string) Decision {
	stages, err := ParsePipeline(line)
	if err != nil {
		return Decision{Verdict: Refuse, Reason: err.Error()}
	}

	d := Decision{Key: Key(stages[0]), Argv: stages[0], Stages: stages}

	// The strictest stage wins: `git status | sudo tee x` is refused on its second.
	verdict := Allow
	reason := ""
	for _, argv := range stages {
		stageVerdict, stageReason := p.decideOne(argv)
		if stageVerdict < verdict {
			verdict, reason = stageVerdict, stageReason
		}
	}
	d.Verdict = verdict
	d.Reason = reason
	if reason == "" {
		d.Reason = "every stage reads and changes nothing"
	}
	return d
}

// decideOne judges a single command.
func (p Policy) decideOne(argv []string) (Verdict, string) {
	key := Key(argv)
	program := programName(argv[0])

	// Checked against both spellings so neither slips past.
	for _, refused := range p.Refused {
		if matches(refused, key, program) {
			return Refuse, fmt.Sprintf("%s is on the refused list", refused)
		}
	}

	// Before the allowlist, so --yes waives confinement too.
	if p.AllowAll {
		return Allow, ""
	}
	for _, allowed := range p.Allowed {
		if matches(allowed, key, program) {
			// An allowance is about the program, so what it is pointed at is
			// still a question: `cat` reads, `cat ~/.ssh/id_rsa` reads that.
			return p.confine(argv)
		}
	}
	return Confirm, fmt.Sprintf("%s is not on the allowed list", key)
}

// matches: an entry with a subcommand matches only it, a bare program matches all.
func matches(entry, key, program string) bool {
	entry = strings.TrimSpace(entry)
	if entry == "" {
		return false
	}
	return entry == key || entry == program
}

// Wildcards are left literal: expanding them broke every tool taking a pattern.

// Result is what running a command produced.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Run executes an already-decided command: argv, not a string, so nothing is re-parsed.
func Run(ctx context.Context, argv []string, timeout time.Duration) (Result, error) {
	return RunIn(ctx, "", argv, timeout)
}

// RunPipelineIn chains stages through os.Pipe, collecting stderr from all of them.
func RunPipelineIn(ctx context.Context, dir string, stages [][]string, timeout time.Duration) (Result, error) {
	if len(stages) == 0 {
		return Result{}, fmt.Errorf("empty command")
	}
	if len(stages) == 1 {
		return RunIn(ctx, dir, stages[0], timeout)
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	var stdout, stderr strings.Builder
	cmds := make([]*exec.Cmd, 0, len(stages))
	for _, argv := range stages {
		// #nosec G204 -- every stage allowlisted by Decide; the pipe is os.Pipe, not a shell.
		// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
		cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
		cmd.Dir = dir
		cmd.Stderr = &stderr
		cmds = append(cmds, cmd)
	}
	for i := 0; i < len(cmds)-1; i++ {
		pipe, err := cmds[i].StdoutPipe()
		if err != nil {
			return Result{}, err
		}
		cmds[i+1].Stdin = pipe
	}
	cmds[len(cmds)-1].Stdout = &stdout

	for _, cmd := range cmds {
		if err := cmd.Start(); err != nil {
			return Result{Stderr: strings.TrimSpace(stderr.String())}, err
		}
	}

	// The last stage's code, as a shell would report.
	var lastErr error
	for _, cmd := range cmds {
		lastErr = cmd.Wait()
	}

	result := Result{
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
	}
	if lastErr != nil {
		var exitErr *exec.ExitError
		if errors.As(lastErr, &exitErr) {
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, lastErr
	}
	return result, nil
}

// RunIn is Run against a given directory, not wherever the process happens to be.
func RunIn(ctx context.Context, dir string, argv []string, timeout time.Duration) (Result, error) {
	if len(argv) == 0 {
		return Result{}, fmt.Errorf("empty command")
	}
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// #nosec G204 -- Decide allowlisted argv[0]; ParseArgv refused shell syntax.
	// nosemgrep: go.lang.security.audit.dangerous-exec-command.dangerous-exec-command
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	result := Result{
		Stdout: strings.TrimSpace(stdout.String()),
		Stderr: strings.TrimSpace(stderr.String()),
	}

	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			// A non-zero exit is an answer: `grep` says "found nothing" this way.
			result.ExitCode = exitErr.ExitCode()
			return result, nil
		}
		return result, err
	}
	return result, nil
}
