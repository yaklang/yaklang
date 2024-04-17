package yak2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/memedit"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type astbuilder struct {
	*ssa.FunctionBuilder
}

func Build(src string, force bool, builder *ssa.FunctionBuilder) error {
	ast, err := FrontEnd(src, force)
	if err != nil {
		return err
	}

	originEditor := builder.Editor
	defer func() {
		builder.Editor = originEditor
	}()

	// include source code will change the context of the origin editor
	newCodeEditor := memedit.NewMemEditor(src)
	originEditor.PushSourceCodeContext(newCodeEditor.SourceCodeMd5())

	builder.Editor = newCodeEditor
	astBuilder := &astbuilder{
		FunctionBuilder: builder,
	}
	astBuilder.build(ast)
	return nil
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
	return nil, utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrors())
}
