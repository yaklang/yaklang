package js2ssa

import (
	_ "embed"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"testing"
	"time"
)

//go:embed test.js
var largeJS string

//go:embed test-seq.js
var largeExprSeqJS string

// func checkLarge(t *testing.T, code string) {
// 	prog := ParseSSA(code, none)
// 	prog.ShowWithSource()
// }

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
