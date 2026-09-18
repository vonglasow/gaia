package execpolicy

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// WritingFlags turn a command that reads into one that writes or runs something else.
//
// An allowlist of programs assumes a program does one thing. `find` reads until
// it is given -delete, and then it is the most destructive tool on the list.
var WritingFlags = []string{
	"-delete", "-exec", "-execdir", "-ok", "-okdir",
	"-fprint", "-fprintf", "-fls", "-in-place", "--in-place",
}

// writingFlag returns the first flag that changes what a command does, or "".
func writingFlag(argv []string) string {
	for _, arg := range argv[1:] {
		name := arg
		if i := strings.IndexByte(name, '='); i >= 0 {
			name = name[:i]
		}
		for _, flag := range WritingFlags {
			if name == flag {
				return flag
			}
		}
		// -i alone is sed's in-place; -i with a suffix is the same thing.
		if name == "-i" && programName(argv[0]) == "sed" {
			return "-i"
		}
	}
	return ""
}

// escapingPath returns the first argument that names somewhere outside the
// workspace, or "".
//
// An allowlist judges the program, not what it is pointed at, so `cat` is
// allowed and `cat ~/.ssh/id_rsa` was too. Only arguments that are already
// paths are judged: a pattern like *.go or a flag is not one, and treating it
// as one would ask about every command.
func escapingPath(workspace string, argv []string) string {
	if strings.TrimSpace(workspace) == "" {
		return ""
	}
	root, err := filepath.EvalSymlinks(workspace)
	if err != nil {
		root = filepath.Clean(workspace)
	}

	for _, arg := range argv[1:] {
		if strings.HasPrefix(arg, "~") {
			return arg
		}
		candidate := arg
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(root, candidate)
		}
		// An argument is a path if it reads like one or if something is there:
		// `ls escape` names a symlink, and its shape says nothing about that.
		if !looksLikeAPath(arg) && !exists(candidate) {
			continue
		}
		candidate = filepath.Clean(candidate)
		if resolved, resolveErr := filepath.EvalSymlinks(candidate); resolveErr == nil {
			candidate = resolved
		}
		if candidate != root && !strings.HasPrefix(candidate, root+string(filepath.Separator)) {
			return arg
		}
	}
	return ""
}

// exists reports whether something is at a path, which makes an argument one.
func exists(path string) bool {
	_, err := os.Lstat(path)
	return err == nil
}

// looksLikeAPath reports whether an argument names a place rather than a value.
func looksLikeAPath(arg string) bool {
	if arg == "" || strings.HasPrefix(arg, "-") {
		return false
	}
	if strings.HasPrefix(arg, "~") || filepath.IsAbs(arg) {
		return true
	}
	if strings.Contains(arg, "..") {
		return true
	}
	// A bare word is a subcommand or a value until something on disk says otherwise.
	if !strings.ContainsRune(arg, os.PathSeparator) {
		return false
	}
	return !strings.ContainsAny(arg, "*?[")
}

// confine narrows an Allow when the command reaches further than a read.
func (p Policy) confine(argv []string) (Verdict, string) {
	if flag := writingFlag(argv); flag != "" {
		return Confirm, fmt.Sprintf("%s makes %s write rather than read", flag, programName(argv[0]))
	}
	if path := escapingPath(p.Workspace, argv); path != "" {
		return Confirm, fmt.Sprintf("%s is outside %s", path, p.Workspace)
	}
	return Allow, ""
}
