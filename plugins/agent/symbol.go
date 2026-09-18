package agent

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"strings"
)

// Reading a file to change one function costs the whole file, every turn it
// stays in the conversation. Go's parser knows where the function starts and
// ends, so ask for it by name.

// findSymbol returns the source of a top-level declaration, with the doc
// comment above it and the line it starts at.
func findSymbol(content, name string) (source string, line int, err error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "", content, parser.ParseComments|parser.SkipObjectResolution)
	if err != nil {
		return "", 0, fmt.Errorf("cannot read this file as Go: %s", firstParseError(err))
	}

	lines := strings.Split(content, "\n")
	for _, decl := range file.Decls {
		start, end, found := declRange(decl, name)
		if !found {
			continue
		}
		from := fset.Position(start).Line
		to := fset.Position(end).Line
		if from < 1 || to > len(lines) {
			return "", 0, fmt.Errorf("%q is in this file but its position is not", name)
		}
		return numbered(lines[from-1:to], from), from, nil
	}
	return "", 0, fmt.Errorf("there is no %q in this file", name)
}

// declRange finds where a named declaration begins, doc comment included, and ends.
func declRange(decl ast.Decl, name string) (start, end token.Pos, found bool) {
	switch d := decl.(type) {
	case *ast.FuncDecl:
		if d.Name.Name != name && methodName(d) != name {
			return 0, 0, false
		}
		return docStart(d.Doc, d.Pos()), d.End(), true
	case *ast.GenDecl:
		for _, spec := range d.Specs {
			if specName(spec) == name {
				return docStart(d.Doc, d.Pos()), d.End(), true
			}
		}
	}
	return 0, 0, false
}

// methodName is how a method is asked for: Type.Method.
func methodName(fn *ast.FuncDecl) string {
	if fn.Recv == nil || len(fn.Recv.List) == 0 {
		return ""
	}
	return receiverType(fn.Recv.List[0].Type) + "." + fn.Name.Name
}

func receiverType(expr ast.Expr) string {
	if star, ok := expr.(*ast.StarExpr); ok {
		expr = star.X
	}
	if ident, ok := expr.(*ast.Ident); ok {
		return ident.Name
	}
	return ""
}

func specName(spec ast.Spec) string {
	switch s := spec.(type) {
	case *ast.TypeSpec:
		return s.Name.Name
	case *ast.ValueSpec:
		if len(s.Names) > 0 {
			return s.Names[0].Name
		}
	}
	return ""
}

// docStart includes the comment above a declaration: it is usually why the
// declaration is the way it is.
func docStart(doc *ast.CommentGroup, fallback token.Pos) token.Pos {
	if doc != nil {
		return doc.Pos()
	}
	return fallback
}

// numbered matches what read_file shows, so a quote copied from either works.
func numbered(lines []string, from int) string {
	var b strings.Builder
	for i, line := range lines {
		fmt.Fprintf(&b, "%d\t%s\n", from+i, line)
	}
	return strings.TrimRight(b.String(), "\n")
}
