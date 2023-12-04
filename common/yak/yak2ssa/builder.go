package yak2ssa

import (
	"fmt"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

type astbuilder struct {
	*ssa.FunctionBuilder
}

type builder struct {
	ast  *yak.ProgramContext
	prog *ssa.Program
	// symbolTable map[string]any
	callback func(*ssa.FunctionBuilder)
}

// build implements ssa.builder.
func (b *builder) Build() {
	pkg := ssa.NewPackage("main")
	b.prog.AddPackage(pkg)
	main := pkg.NewFunction("yak-main")
	funcBuilder := ssa.NewBuilder(main, nil)
	if b.callback != nil {
		b.callback(funcBuilder)
	}

	astbuilder := astbuilder{
		FunctionBuilder: funcBuilder,
	}
	astbuilder.build(b.ast)
	astbuilder.Finish()
}

var _ (ssa.Builder) = (*builder)(nil)

// error listener for lexer and parser
// type position struct {
// 	line int
// 	col  int
// }

//	type astErr struct {
//		msg   string
//		start position
//		end   position
//	}
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

func ParseSSA(src string, f func(*ssa.FunctionBuilder)) (prog *ssa.Program) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recover from yak2ssa.ParseSSA: ", r)
			// fmt.Println("\n\n\n!!!!!!!\n\n!!!!!\n\nRecovered in parseSSA", r)
			// debug.PrintStack()
			prog = nil
		}
	}()

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
	if len(errListener.err) > 0 {
		return nil
	}
	// yak.NewProgramContext(ast, )
	prog = ssa.NewProgram()
	builder := &builder{
		ast:      ast,
		prog:     prog,
		callback: f,
	}
	prog.Build(builder)
	ssa4analyze.NewAnalyzerGroup(
		prog,
	).Run()
	return prog
}
