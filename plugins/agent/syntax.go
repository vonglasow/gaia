package agent

import (
	"fmt"
	"go/parser"
	"go/token"
	"strings"
)

// An edit that leaves a file unparseable is worse than one that is refused: the
// model moves on, the build breaks somewhere else, and the cause is three turns
// back. Go's parser is in the standard library, so checking costs nothing.

// checkSyntax reports why content would not compile, for the languages we can
// read. Anything else is accepted as-is.
func checkSyntax(path, content string) error {
	if !strings.HasSuffix(path, ".go") {
		return nil
	}
	_, err := parser.ParseFile(token.NewFileSet(), path, content, parser.SkipObjectResolution)
	if err == nil {
		return nil
	}
	return fmt.Errorf("that would leave %s unparseable: %s%s", path, firstParseError(err), escapeHint(err))
}

func firstParseError(err error) string {
	first, _, _ := strings.Cut(err.Error(), "\n")
	return strings.TrimSpace(first)
}

// escapeHint names the mistake behind most of these: a \n meant for Go source
// arrives as a real newline, because the arguments were JSON first.
func escapeHint(err error) string {
	text := err.Error()
	if !strings.Contains(text, "newline in string") && !strings.Contains(text, "string literal not terminated") {
		return ""
	}
	return `. A \n inside a Go string literal must be written \\n in your arguments, ` +
		`or it becomes a real line break`
}
