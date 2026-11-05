package pythonparser

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
)

func parsePythonSourceForTest(src string) error {
	_, err := antlr4util.ParseASTWithSLLFirst(
		src,
		func(input antlr.CharStream) *PythonLexer { return NewPythonLexer(input) },
		NewPythonParser,
		nil,
		nil,
		func(p *PythonParser) IRootContext { return p.Root() },
	)
	return err
}

// A whitespace-only final line is a blank line. The lexer must not turn its
// indentation into an INDENT token at EOF.
func TestPythonLexerTrailingWhitespaceAtEOF(t *testing.T) {
	require.NoError(t, parsePythonSourceForTest("x = 1\n    "))
	require.NoError(t, parsePythonSourceForTest("x = 1\n    \n"))
	require.NoError(t, parsePythonSourceForTest("if True:\n    x = 1\n    "))
}

// A backslash continuation may start at column zero. Its first token must not
// leak the "line join" state into the indentation of the next statement.
func TestPythonLexerLineJoinColumnZero(t *testing.T) {
	src := "def f():\n    html = \\\n'''a'''\n    res = \\\nb'''c'''\n    return html, res\n"
	require.NoError(t, parsePythonSourceForTest(src))
}

// A continuation line indented deeper than the statement must stay a
// continuation, not a new suite.
func TestPythonLexerLineJoinIndentedContinuation(t *testing.T) {
	src := "def f():\n    x = 1 + \\\n        2\n    return x\n"
	require.NoError(t, parsePythonSourceForTest(src))
}
