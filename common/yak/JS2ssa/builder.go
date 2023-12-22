package js2ssa

import (
	"fmt"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"

	"github.com/yaklang/yaklang/common/log"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

type Parser struct {
}

func NewParser() *Parser {
	return &Parser{}
}

func (p *Parser) Parse(src string, must bool, callBack func(*ssa.FunctionBuilder)) *ssa.Program {
	return parseSSA(src, must, nil, callBack)
}

type astbuilder struct {
	*ssa.FunctionBuilder
}

func parseSSA(src string, force bool, prog *ssa.Program, callback func(*ssa.FunctionBuilder)) (ret *ssa.Program) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recover from js2ssa.parseSSA: ", r)
			// fmt.Println("\n\n\n!!!!!!!\n\n!!!!!\n\nRecovered in parseSSA", r)
			// debug.PrintStack()
			ret = nil
		}
	}()

	frontend(src, force, func(ast *JS.ProgramContext) {
		if prog == nil {
			prog = ssa.NewProgram()
		}
		funcBuilder := prog.GetAndCreateMainFunctionBuilder()
		if funcBuilder == nil {
			return
		}
		if callback != nil {
			callback(funcBuilder)
		}
		astbuilder := astbuilder{
			FunctionBuilder: funcBuilder,
		}
		astbuilder.build(ast)
		astbuilder.Finish()
	})

	ssa4analyze.RunAnalyzer(prog)
	return prog
}

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
		err:                  []string{},
		DefaultErrorListener: antlr.NewDefaultErrorListener(),
	}
}

func frontend(src string, must bool, handler func(*JS.ProgramContext)) {
	errListener := NewErrorListener()
	// start := time.Now()
	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	// log.Info(time.Since(start))
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	ast := parser.Program().(*JS.ProgramContext)
	// log.Info("ast time ", time.Since(start))
	if must || len(errListener.err) == 0 {
		handler(ast)
	} else {
		log.Info(errListener.err)
	}
}
