package go2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	goparser "github.com/yaklang/yaklang/common/yak/go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type builder struct {
	ir       *ssa.FunctionBuilder
	constMap map[string]ssa.Value
}

func (a *builder) Build(src string, force bool, b *ssa.FunctionBuilder) error {
	ast, err := a.FrondEnd(src, force)
	if err != nil {
		return err
	}
	b.DisableFreeValue = true
	build := builder{ir: b}
	build.VisitSourceFile(ast)
	return nil
}
func (a *builder) FrondEnd(src string, force bool) (goparser.ISourceFileContext, error) {
	listener := antlr4util.NewErrorListener()
	lexer := goparser.NewGoLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(listener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := goparser.NewGoParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(listener)
	source := parser.SourceFile()
	if force || len(listener.GetErrors()) == 0 {
		return source, nil
	}
	return nil, utils.Errorf("parse ast frontend error: %s", listener.GetErrors())
}
