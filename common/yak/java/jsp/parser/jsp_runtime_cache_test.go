package jspparser

import (
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/yaklang/antlr/v4"
)

func TestResetJSPRuntimeCachesDropsDFA(t *testing.T) {
	ResetJSPRuntimeCaches()
	require.Zero(t, JSPRuntimeDFAStates())

	lexer := NewJSPLexer(antlr.NewInputStream(`<html><body>hello</body></html>`))
	parser := NewJSPParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	parser.RemoveErrorListeners()
	_ = parser.JspDocuments()

	require.Greater(t, JSPRuntimeDFAStates(), 0, "a parse must retain shared DFA states")
	ResetJSPRuntimeCaches()
	require.Zero(t, JSPRuntimeDFAStates(), "reset must drop the shared DFA, not leave it for the next phase")
}
