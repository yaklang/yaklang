package yak2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

type Parser struct {
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(code string, must bool, callBack func(*ssa.FunctionBuilder)) (*ssa.Program, error) {
	return parseSSA(code, must, nil, callBack)
}

func (p *Parser) Feed(code string, ignoreSyntax bool, prog *ssa.Program) {
	parseSSA(code, ignoreSyntax, prog, nil)
}

type astbuilder struct {
	*ssa.FunctionBuilder
}

func parseSSA(src string, force bool, prog *ssa.Program, callback func(*ssa.FunctionBuilder)) (ret *ssa.Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			// debug.PrintStack()
			ret = nil
			err = utils.Errorf("parse error with panic : %v", r)
		}
	}()

	if err := frontEnd(src, force, func(ast *yak.ProgramContext) {
		if prog == nil {
			prog = ssa.NewProgram()
		}
		builder := prog.GetAndCreateMainFunctionBuilder()
		if callback != nil {
			callback(builder)
		}
		astbuilder := astbuilder{
			FunctionBuilder: builder,
		}
		astbuilder.build(ast)
		astbuilder.Finish()
	}); err != nil {
		return nil, err
	}

	ssa4analyze.RunAnalyzer(prog)
	return prog, nil
}

func frontEnd(src string, must bool, callback func(ast *yak.ProgramContext)) error {
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
		callback(ast)
		return nil
	} else {
		return utils.Errorf("parse AST FrontEnd error : %v", errListener.GetErrors())
	}
}
