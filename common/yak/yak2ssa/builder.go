package yak2ssa

import (
	"path/filepath"

	"github.com/yaklang/yaklang/common/consts"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type singleFileBuilder struct {
	*ssa.PreHandlerInit
}

var _ ssa.Builder = (*singleFileBuilder)(nil)
var Builder = &singleFileBuilder{}

func (s *singleFileBuilder) Create() ssa.Builder {
	return &singleFileBuilder{
		PreHandlerInit: ssa.NewPreHandlerInit().WithLanguageConfigOpts(
			ssa.WithLanguageConfigShouldBuild(func(filename string) bool {
				return true
			}),
			ssa.WithLanguageBuilder(s),
		),
	}
}

func (*singleFileBuilder) ParseAST(src string) (ssa.FrontAST, error) {
	return FrontEnd(src)
}

func (*singleFileBuilder) BuildFromAST(ast ssa.FrontAST, b *ssa.FunctionBuilder) error {
	b.SupportClosure = true
	astBuilder := &astbuilder{
		FunctionBuilder: b,
	}
	astBuilder.build(ast)
	return nil
}

func (*singleFileBuilder) FilterFile(path string) bool {
	a := filepath.Ext(path)
	_ = a
	return filepath.Ext(path) == ".yak"
}
func (*singleFileBuilder) FilterPreHandlerFile(path string) bool {
	return filepath.Ext(path) == ".yak" || filepath.Ext(path) == ".yaklang"
}

func (*singleFileBuilder) GetLanguage() consts.Language {
	return consts.Yak
}

type astbuilder struct {
	*ssa.FunctionBuilder
}

func FrontEnd(src string) (yak.IProgramContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := yak.NewYaklangParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.Program()
	var err error
	if len(errListener.GetErrors()) != 0 {
		err = utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrorString())
	}
	return ast, err
}
