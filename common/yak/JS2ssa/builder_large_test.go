package js2ssa

import (
	_ "embed"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/esparser"
	"testing"
)

//go:embed test.js
var largeJS string

//go:embed test-seq.js
var largeExprSeqJS string

func checkLarge(t *testing.T, code string) {
	prog := ParseSSA(code, none)
	prog.ShowWithSource()
}

func TestJS_ASTLargeText(t *testing.T) {
	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(largeExprSeqJS))
	lexer.RemoveErrorListeners()
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	_ = parser.Program()
}
