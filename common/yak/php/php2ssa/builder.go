package php2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type builder struct {
	ast  *phpparser.HtmlDocumentContext
	prog *ssa.Program
}

func ParseSSA(src string, f func(builder *ssa.FunctionBuilder)) (prog *ssa.Program) {
	defer func() {
		if r := recover(); r != nil {
			// fmt.Println("recover from php2ssa.ParseSSA: ", r)
		}
	}()

	lex := phpparser.NewPHPLexer(antlr.NewInputStream(src))
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	parser := phpparser.NewPHPParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(antlr4util.NewLegacyErrorListener())
	builder := &builder{
		prog: ssa.NewProgram(),
	}
	builder.VisitHtmlDocument(parser.HtmlDocument())
	return builder.prog
}
