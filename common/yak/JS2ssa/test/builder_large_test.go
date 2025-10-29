package test

import (
	_ "embed"
	"testing"
	"time"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/stretchr/testify/require"
	"github.com/yaklang/yaklang/common/log"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
)

//go:embed test.js
var largeJS string

//go:embed test-seq.js
var largeExprSeqJS string

func TestJS_MVP_ASTLargeText(t *testing.T) {
	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(largeExprSeqJS))
	lexer.RemoveErrorListeners()
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	_ = parser.Program()
}

func TestJS_ASTLargeText(t *testing.T) {
	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(largeJS))
	lexer.RemoveErrorListeners()
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	start := time.Now()
	ts := tokenStream.GetTokenSource()
	count := 0
	for {
		t := ts.NextToken()
		count++
		_ = t
		if count%1000 == 0 {
		}
		if t.GetTokenType() == antlr.TokenEOF {
			break
		}
	}
	log.Infof("get all tokens cost: %v", time.Now().Sub(start))

	log.Infof("start to build ast via parser")
	lexer = JS.NewJavaScriptLexer(antlr.NewInputStream(largeJS))
	lexer.RemoveErrorListeners()
	tokenStream = antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	_ = parser.Program()
	log.Infof("finish to build ast via parser")

}

func TestRealJsFrontEnd(t *testing.T) {
	ast, err := js2ssa.Frontend(largeJS)
	require.Nil(t, err)
	_ = ast
}

// func TestRealJs(t *testing.T) {
// 	prog, err := ssaapi.Parse(largeJS, ssaapi.WithLanguage(ssaconfig.JS))
// 	if err != nil {
// 		t.Fatal("prog parse error", err)
// 	}
// 	_ = prog
// }
