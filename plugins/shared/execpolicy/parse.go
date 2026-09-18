// Package execpolicy judges a model's command line, and runs it without a shell.
package execpolicy

import (
	"fmt"
	"strings"
	"unicode"
)

// Characters a shell interprets and exec does not: refused rather than passed through.
const shellMetacharacters = "&;<>()$`\\\"'\n\r"

// ParsePipeline splits on pipes: a pipe connects programs, it does not create one.
func ParsePipeline(line string) ([][]string, error) {
	var stages [][]string
	for _, segment := range splitOnPipes(line) {
		argv, err := ParseArgv(segment)
		if err != nil {
			return nil, err
		}
		stages = append(stages, argv)
	}
	if len(stages) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return stages, nil
}

// splitOnPipes cuts at every pipe outside quotes, so `grep "a|b"` stays one stage.
func splitOnPipes(line string) []string {
	var (
		segments []string
		current  strings.Builder
		quote    rune
	)
	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			}
			current.WriteRune(r)
		case r == '\'' || r == '"':
			quote = r
			current.WriteRune(r)
		case r == '|':
			segments = append(segments, current.String())
			current.Reset()
		default:
			current.WriteRune(r)
		}
	}
	segments = append(segments, current.String())
	return segments
}

// ParseArgv splits one command, refusing anything a shell would have interpreted.
func ParseArgv(line string) ([]string, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, fmt.Errorf("empty command")
	}

	var (
		argv    []string
		current strings.Builder
		quote   rune
		started bool
	)

	for _, r := range line {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
				continue
			}
			current.WriteRune(r)
			started = true
		case r == '\'' || r == '"':
			quote = r
			started = true
		case strings.ContainsRune(shellMetacharacters, r):
			// Before the whitespace case: a newline is both, and the shell reading wins.
			return nil, fmt.Errorf(
				"refusing %q: %q is shell syntax, and commands here do not go through a shell. "+
					"Run one command per call and read its output; to filter or count, use the "+
					"command's own flags (find -name, grep -c, ls -1) rather than a pipe",
				line, string(r))
		case unicode.IsSpace(r):
			if started {
				argv = append(argv, current.String())
				current.Reset()
				started = false
			}
		default:
			current.WriteRune(r)
			started = true
		}
	}

	if quote != 0 {
		return nil, fmt.Errorf("refusing %q: unbalanced quote", line)
	}
	if started {
		argv = append(argv, current.String())
	}
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty command")
	}
	return argv, nil
}

// Key names a command for a list: program, plus a first argument reading as a subcommand.
func Key(argv []string) string {
	if len(argv) == 0 {
		return ""
	}
	program := programName(argv[0])
	if len(argv) == 1 {
		return program
	}
	if isSubcommand(argv[1]) {
		return program + " " + argv[1]
	}
	return program
}

// programName strips the directory, so a spelled-out path cannot sidestep a list entry.
func programName(path string) string {
	if i := strings.LastIndexAny(path, "/\\"); i >= 0 {
		return path[i+1:]
	}
	return path
}

// isSubcommand reports whether an argument reads as a verb, not a flag or a path.
func isSubcommand(arg string) bool {
	if arg == "" || strings.HasPrefix(arg, "-") {
		return false
	}
	if strings.ContainsAny(arg, "/\\.") {
		return false
	}
	for _, r := range arg {
		if !unicode.IsLetter(r) && r != '-' && r != '_' {
			return false
		}
	}
	return true
}
