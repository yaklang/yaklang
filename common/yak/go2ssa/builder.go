package go2ssa

import (
	"path/filepath"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/ssa"

	gol "github.com/yaklang/yaklang/common/yak/antlr4go/parser"
)

type SSABuild struct {
	ssa.DummyExtraFileAnalyzer
}

var Builder = &SSABuild{}

func (*SSABuild) Build(src string, force bool, builder *ssa.FunctionBuilder) error {
	ast, err := Frontend(src, force)
	if err != nil {
		return err
	}
	builder.SupportClosure = true
	astBuilder := &astbuilder{
		FunctionBuilder: builder,
		lmap:            make(map[string]struct{}),
		cmap:            make(map[string]struct{}),
	}
	log.Infof("ast: %s", ast.ToStringTree(ast.GetParser().GetRuleNames(), ast.GetParser()))
	astBuilder.build(ast)
	return nil
}

func (*SSABuild) FilterFile(path string) bool {
	return filepath.Ext(path) == ".go"
}


type astbuilder struct {
	*ssa.FunctionBuilder
	lmap map[string]struct{}
	cmap map[string]struct{}
}

func Frontend(src string, must bool) (*gol.SourceFileContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := gol.NewGoLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := gol.NewGoParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.SourceFile().(*gol.SourceFileContext)
	if must || len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrorString())
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
