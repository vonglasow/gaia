package agent

import (
	"strings"
)

// A model quotes code from what read_file showed it, and gets the indentation
// wrong: Go files are tabs, and models write spaces. Matching on that would
// make edit_file unusable for the language the project is written in.

// findBlock locates old in content, exactly if it can and ignoring how each line
// is indented if it cannot. It reports the span and whether it was unique.
func findBlock(content, old string) (start, end int, found, unique bool) {
	if i := strings.Index(content, old); i >= 0 {
		if strings.Count(content, old) > 1 {
			return 0, 0, true, false
		}
		return i, i + len(old), true, true
	}
	return findIndented(content, old)
}

// Unescaped says the quote only matched once its \n sequences were read as line
// breaks. The arguments are JSON first, and a model escapes one time too many
// about as often as one time too few.
func findBlockTolerant(content, old string) (start, end int, found, unique, unescaped bool) {
	start, end, found, unique = findBlock(content, old)
	if found {
		return start, end, found, unique, false
	}
	for _, repaired := range []string{unescapeSequences(old), rejoinBrokenStrings(old)} {
		if repaired == old {
			continue
		}
		if start, end, found, unique = findBlock(content, repaired); found {
			return start, end, found, unique, true
		}
	}
	return 0, 0, false, false, false
}

// rejoinBrokenStrings repairs the other direction of the same mistake: a \n
// inside a Go string literal, written once instead of twice, arrives as a real
// line break and splits a line the file keeps whole. A quote line that leaves a
// string open is exactly that, and nothing else.
func rejoinBrokenStrings(text string) string {
	lines := strings.Split(text, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		if len(out) > 0 && unterminatedString(out[len(out)-1]) {
			out[len(out)-1] += `\n` + line
			continue
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

// unterminatedString reports a line leaving a double-quoted string open.
func unterminatedString(line string) bool {
	open, escaped := false, false
	for _, r := range line {
		switch {
		case escaped:
			escaped = false
		case r == '\\':
			escaped = true
		case r == '"':
			open = !open
		}
	}
	return open
}

// unescapeSequences turns literal \n and \t into what they stand for, and only
// in text that has no real line break of its own — otherwise it is Go source
// that legitimately contains them.
func unescapeSequences(text string) string {
	if strings.Contains(text, "\n") || !strings.Contains(text, `\n`) {
		return text
	}
	replaced := strings.ReplaceAll(text, `\n`, "\n")
	return strings.ReplaceAll(replaced, `\t`, "\t")
}

// findIndented compares line by line with the leading whitespace taken off.
func findIndented(content, old string) (start, end int, found, unique bool) {
	fileLines := strings.Split(content, "\n")
	wanted := strings.Split(strings.TrimRight(old, "\n"), "\n")
	if len(wanted) == 0 || len(wanted) > len(fileLines) {
		return 0, 0, false, false
	}

	matches := [][2]int{}
	for i := 0; i+len(wanted) <= len(fileLines); i++ {
		if sameIgnoringIndent(fileLines[i:i+len(wanted)], wanted) {
			matches = append(matches, [2]int{i, i + len(wanted)})
		}
	}
	switch len(matches) {
	case 0:
		return 0, 0, false, false
	case 1:
	default:
		return 0, 0, true, false
	}

	offsets := lineOffsets(fileLines)
	first, last := matches[0][0], matches[0][1]
	start = offsets[first]
	end = offsets[last-1] + len(fileLines[last-1])
	return start, end, true, true
}

func sameIgnoringIndent(have, want []string) bool {
	for i := range want {
		if strings.TrimLeft(have[i], " \t") != strings.TrimLeft(want[i], " \t") {
			return false
		}
	}
	return true
}

func lineOffsets(lines []string) []int {
	offsets := make([]int, len(lines))
	at := 0
	for i, line := range lines {
		offsets[i] = at
		at += len(line) + 1 // the newline split took off
	}
	return offsets
}

// reindent shifts new to sit where old sat, so a replacement written with
// spaces lands in a file written with tabs.
func reindent(newText, replacedFirstLine string) string {
	lines := strings.Split(newText, "\n")
	if len(lines) == 0 {
		return newText
	}
	fileIndent := leadingSpace(replacedFirstLine)
	newIndent := leadingSpace(lines[0])
	if fileIndent == newIndent {
		return newText
	}
	for i, line := range lines {
		if strings.TrimSpace(line) == "" {
			lines[i] = ""
			continue
		}
		lines[i] = fileIndent + strings.TrimPrefix(line, newIndent)
	}
	return strings.Join(lines, "\n")
}

func leadingSpace(line string) string {
	return line[:len(line)-len(strings.TrimLeft(line, " \t"))]
}
