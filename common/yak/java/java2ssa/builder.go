package java2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type builder struct {
	*ssa.FunctionBuilder
	ast      javaparser.ICompilationUnitContext
	constMap map[string]ssa.Value
}

func Build(src string, force bool, b *ssa.FunctionBuilder) error {
	ast, err := Frontend(src, force)
	if err != nil {
		return err
	}
	build := &builder{
		FunctionBuilder: b,
		ast:             ast,
		constMap:        make(map[string]ssa.Value),
	}
	b.DisableFreeValue = false
	build.VisitCompilationUnit(ast)
	return nil
}

func Frontend(src string, force bool) (javaparser.ICompilationUnitContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := javaparser.NewJavaLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.CompilationUnit()
	if force || len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrors())
}
