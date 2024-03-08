package js2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"

	"github.com/yaklang/yaklang/common/utils"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type astbuilder struct {
	*ssa.FunctionBuilder
	lmap map[string]struct{}
	cmap map[string]struct{}
}

func NewAstBuilder(functionBuilder *ssa.FunctionBuilder) *astbuilder {
	return &astbuilder{
		FunctionBuilder: functionBuilder,
		lmap:            make(map[string]struct{}),
		cmap:            make(map[string]struct{}),
	}
}

func Build(src string, force bool, builder *ssa.FunctionBuilder) error {
	ast, err := Frontend(src, force)
	if err != nil {
		return err
	}
	astBuilder := NewAstBuilder(builder)
	astBuilder.build(ast)
	return nil
}

func Frontend(src string, must bool) (*JS.ProgramContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.Program().(*JS.ProgramContext)
	if must || len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrors())
}

func (b *astbuilder) AddToCmap(key string) {
	b.cmap[key] = struct{}{}
}

func (b *astbuilder) GetFromCmap(key string) bool {
	if _, ok := b.cmap[key]; ok {
		return true
	} else {
		return false
	}
}

func (b *astbuilder) AddToLmap(key string) {
	b.lmap[key] = struct{}{}
}

func (b *astbuilder) GetFromLmap(key string) bool {
	if _, ok := b.lmap[key]; ok {
		return true
	} else {
		return false
	}
}
