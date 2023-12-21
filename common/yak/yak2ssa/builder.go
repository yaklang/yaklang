package yak2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"runtime/debug"
)

type Parser struct {
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(code string, must bool, callBack func(*ssa.FunctionBuilder)) *ssa.Program {
	return parseSSA(code, must, nil, callBack)
}

type astbuilder struct {
	*ssa.FunctionBuilder
}

func parseSSA(src string, force bool, prog *ssa.Program, callback func(*ssa.FunctionBuilder)) (ret *ssa.Program) {
	defer func() {
		if r := recover(); r != nil {
			log.Error("recover from yak2ssa.parseSSA: ", r)
			debug.PrintStack()
			ret = nil
		}
	}()

	frontEnd(src, force, func(ast *yak.ProgramContext) {
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
	})

	ssa4analyze.RunAnalyzer(prog)
	return prog
}

// func middleEnd(code string, prog *ssa.Program)

// error listener for lexer and parser
type ErrorListener struct {
	err []string
	*antlr.DefaultErrorListener
}

func (el *ErrorListener) SyntaxError(recognizer antlr.Recognizer, offendingSymbol interface{}, line, column int, msg string, e antlr.RecognitionException) {
	el.err = append(el.err, msg)
}

func NewErrorListener() *ErrorListener {
	return &ErrorListener{
		err:                  make([]string, 0),
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
	}
}

func frontEnd(src string, must bool, callback func(ast *yak.ProgramContext)) {
	errListener := NewErrorListener()
	lexer := yak.NewYaklangLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := yak.NewYaklangParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.Program().(*yak.ProgramContext)
	if must || len(errListener.err) == 0 {
		callback(ast)
	}
}
