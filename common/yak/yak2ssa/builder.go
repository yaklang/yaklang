package yak2ssa

import (
	"github.com/yaklang/yaklang/common/consts"
	"path/filepath"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type SSABuilder struct {
	*ssa.PreHandlerInit
}

var Builder = &SSABuilder{}

func (s *SSABuilder) Create() ssa.Builder {
	return &SSABuilder{
		PreHandlerInit: ssa.NewPreHandlerInit().WithLanguageConfigOpts(
			ssa.WithLanguageConfigShouldBuild(func(filename string) bool {
				return true
			}),
			ssa.WithLanguageBuilder(s),
		),
	}
}

func (*SSABuilder) Build(src string, force bool, b *ssa.FunctionBuilder) error {
	ast, err := FrontEnd(src, force)
	if err != nil {
		return err
	}
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

func (*SSABuilder) GetLanguage() consts.Language {
	return consts.Yak
}

type astbuilder struct {
	*ssa.FunctionBuilder
}

func FrontEnd(src string, must bool) (*yak.ProgramContext, error) {
	errListener := antlr4util.NewErrorListener()
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := yak.NewYaklangParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.Program().(*yak.ProgramContext)
	if must || len(errListener.GetErrors()) == 0 {
		return ast, nil
	}
	return nil, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrorString())
}
