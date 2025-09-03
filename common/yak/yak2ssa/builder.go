package yak2ssa

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type SSABuilder struct {
	*ssa.PreHandlerBase
}

var _ ssa.Builder = (*SSABuilder)(nil)

// var Builder = &singleFileBuilder{}
func Biulder() *SSABuilder {
	return &SSABuilder{}
}

func CreateBuilder() ssa.Builder {
	builder := &SSABuilder{
		PreHandlerBase: ssa.NewPreHandlerBase(),
	}
	builder.WithLanguageConfigOpts(
		ssa.WithLanguageConfigShouldBuild(func(filename string) bool {
			return true
		}),
		ssa.WithLanguageBuilder(builder),
	)
	return builder
}

func (s *SSABuilder) ParseAST(src string) (ssa.FrontAST, error) {
	return FrontEnd(src, s)
}

func (*SSABuilder) BuildFromAST(ast ssa.FrontAST, b *ssa.FunctionBuilder) error {
	b.SupportClosure = true
	astBuilder := &astbuilder{
		FunctionBuilder: b,
	}
	astBuilder.build(ast)
	return nil
}

func (*SSABuilder) FilterFile(path string) bool {
	a := filepath.Ext(path)
	_ = a
	return filepath.Ext(path) == ".yak"
}
func (*SSABuilder) FilterPreHandlerFile(path string) bool {
	return filepath.Ext(path) == ".yak" || filepath.Ext(path) == ".yaklang"
}

func (*SSABuilder) GetLanguage() consts.Language {
	return consts.Yak
}

type astbuilder struct {
	*ssa.FunctionBuilder
}

func FrontEnd(src string, ssabuilder ...*SSABuilder) (yak.IProgramContext, error) {
	var builder *ssa.PreHandlerBase
	if len(ssabuilder) > 0 {
		builder = ssabuilder[0].PreHandlerBase
	}
	errListener := antlr4util.NewErrorListener()
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := yak.NewYaklangParser(tokenStream)
	ssa.ParserSetAntlrCache(parser.BaseParser, builder)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.Program()
	return ast, errListener.Error()
}
