package php2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type builder struct {
	ir *ssa.FunctionBuilder
}

func Build(src string, force bool, b *ssa.FunctionBuilder) error {
	ast, err := frondEnd(src, force)
	if err != nil {
		return err
	}
	build := builder{
		ir: b,
	}
	build.VisitHtmlDocument(ast)
	return nil
}

func frondEnd(src string, force bool) (phpparser.IHtmlDocumentContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := phpparser.NewPHPLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := phpparser.NewPHPParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.HtmlDocument()
	if force || len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrors())
}
