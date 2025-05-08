package sfvm

import (
	"fmt"
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/syntaxflow/sf"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"testing"
)

func compileSyntaxFlow(text string) *SyntaxFlowVisitor {
	var errs antlr4util.SourceCodeErrors
	errHandler := antlr4util.SimpleSyntaxErrorHandler(func(msg string, start, end memedit.PositionIf) {
		errs = append(errs, antlr4util.NewSourceCodeError(msg, start, end))
	})
	errLis := antlr4util.NewErrorListener(func(self *antlr4util.ErrorListener, recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
		antlr4util.StringSyntaxErrorHandler(self, recognizer, offendingSymbol, line, column, msg, e)
		errHandler(self, recognizer, offendingSymbol, line, column, msg, e)
	})
	lexer := sf.NewSyntaxFlowLexer(antlr.NewInputStream(text))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errLis)
	astParser := sf.NewSyntaxFlowParser(antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel))
	astParser.RemoveErrorListeners()
	astParser.AddErrorListener(errLis)
	visitor := NewSyntaxFlowVisitor()
	visitor.VisitFlow(astParser.Flow())
	return visitor
}
func TestName(t *testing.T) {
	syntaxFlow := compileSyntaxFlow(`println(* #-> * as $param)`)
	for _, code := range syntaxFlow.codes {
		fmt.Println(Opcode2String[code.OpCode])
	}
}
