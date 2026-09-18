package agent

import (
	"testing"

	"github.com/stretchr/testify/require"
)

// Go files are indented with tabs and models quote them with spaces. Matching
// on that would make edit_file unusable for the language this project is in.

const aFunction = "func Build(items []Item) string {\n" +
	"\tif len(items) == 0 {\n" +
	"\t\treturn raw\n" +
	"\t}\n" +
	"\treturn joined\n" +
	"}\n"

func TestAnExactQuoteIsFound(t *testing.T) {
	start, end, found, unique := findBlock(aFunction, "\t\treturn raw\n")

	require.True(t, found)
	require.True(t, unique)
	require.Equal(t, "\t\treturn raw\n", aFunction[start:end])
}

func TestAQuoteWrittenWithSpacesFindsTheTabbedLine(t *testing.T) {
	start, end, found, unique := findBlock(aFunction, "    if len(items) == 0 {\n        return raw")

	require.True(t, found)
	require.True(t, unique)
	require.Equal(t, "\tif len(items) == 0 {\n\t\treturn raw", aFunction[start:end])
}

func TestAQuoteWithNoIndentAtAllStillFindsIt(t *testing.T) {
	_, _, found, unique := findBlock(aFunction, "return joined")

	require.True(t, found)
	require.True(t, unique)
}

// Ambiguity is refused whichever way the match was made: replacing the wrong
// one of three would be silent.
func TestTextAppearingTwiceIsNotUnique(t *testing.T) {
	content := "\tx := 1\n\ty := 2\n\tx := 1\n"

	_, _, found, unique := findBlock(content, "x := 1")

	require.True(t, found)
	require.False(t, unique)
}

func TestTextAppearingTwiceOnlyAfterTrimmingIsAlsoRefused(t *testing.T) {
	content := "\tif ok {\n\t}\n        if ok {\n        }\n"

	_, _, found, unique := findBlock(content, "if ok {")

	require.True(t, found)
	require.False(t, unique)
}

func TestTextThatIsNotThereIsNotFound(t *testing.T) {
	_, _, found, _ := findBlock(aFunction, "func Other()")

	require.False(t, found)
}

func TestAQuoteLongerThanTheFileIsNotFound(t *testing.T) {
	_, _, found, _ := findBlock("one line\n", "a\nb\nc\nd\ne\n")

	require.False(t, found)
}

// The replacement lands where the old text sat, in the file's own indentation.
func TestTheReplacementTakesTheIndentationOfWhatItReplaces(t *testing.T) {
	got := reindent("if len(items) == 0 {\n    return \"\"\n}", "\t\tif len(items) == 0 {")

	require.Equal(t, "\t\tif len(items) == 0 {\n\t\t    return \"\"\n\t\t}", got)
}

func TestMatchingIndentationIsLeftAlone(t *testing.T) {
	require.Equal(t, "\tx := 1", reindent("\tx := 1", "\ty := 2"))
}

func TestBlankLinesStayBlankRatherThanCarryingIndentation(t *testing.T) {
	got := reindent("a\n\nb", "\t\tz")

	require.Equal(t, "\t\ta\n\n\t\tb", got)
}

// The arguments are JSON before they are Go, and a model escapes one time too
// many about as often as one time too few. Both were seen in the same session.
func TestAQuoteWithLiteralEscapeSequencesStillMatches(t *testing.T) {
	_, _, found, unique, unescaped := findBlockTolerant(aFunction, `if len(items) == 0 {\n\t\treturn raw`)

	require.True(t, found)
	require.True(t, unique)
	require.True(t, unescaped, "and says it had to read them as line breaks")
}

func TestAQuoteThatMatchesAsWrittenIsNotUnescaped(t *testing.T) {
	_, _, found, _, unescaped := findBlockTolerant(aFunction, "\treturn joined")

	require.True(t, found)
	require.False(t, unescaped)
}

// Go source legitimately contains \n, and text with real line breaks is already
// what it says it is.
func TestTextWithRealLineBreaksIsLeftAlone(t *testing.T) {
	given := "fmt.Println(\"a\\nb\")\nreturn nil"

	require.Equal(t, given, unescapeSequences(given))
}

func TestTextWithNoEscapesAtAllIsLeftAlone(t *testing.T) {
	require.Equal(t, "return nil", unescapeSequences("return nil"))
}

// A \n inside a Go string literal, written once instead of twice, arrives as a
// real line break and splits a line the file keeps whole. It is what a model
// writes when quoting any line that formats something.
func TestAQuoteWhoseStringLiteralWasBrokenAcrossLinesStillMatches(t *testing.T) {
	file := "func f() string {\n\tif x {\n\t\treturn \"Header:\\n\" + body()\n\t}\n\treturn \"\"\n}\n"
	// What the model sends once JSON has decoded its single-escaped \n.
	quote := "\tif x {\n\t\treturn \"Header:\n\" + body()\n\t}"

	_, _, found, unique, repaired := findBlockTolerant(file, quote)

	require.True(t, found)
	require.True(t, unique)
	require.True(t, repaired)
}

func TestALineThatClosesItsStringsIsNotRejoined(t *testing.T) {
	given := "\treturn \"a\" + \"b\"\n\treturn nil"

	require.Equal(t, given, rejoinBrokenStrings(given))
}

func TestAnEscapedQuoteDoesNotCountAsOpening(t *testing.T) {
	require.False(t, unterminatedString(`x := "a \" b"`))
	require.True(t, unterminatedString(`x := "a `))
	require.False(t, unterminatedString(`x := 1`))
}
