package js2ssa

import (
	"fmt"
	"time"

	"github.com/antlr/antlr4/runtime/Go/antlr/v4"

	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

type astbuilder struct {
	*ssa.FunctionBuilder
}

type builder struct {
	ast      *JS.ProgramContext
	prog     *ssa.Program
	callback func(*ssa.FunctionBuilder)
}

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

func ParseSSA(src string, f func(*ssa.FunctionBuilder)) (prog *ssa.Program) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recover from js2ssa.ParseSSA: ", r)
			// fmt.Println("Recovered in parseSSA", r)
			// debug.PrintStack()
			prog = nil
		}
	}()

	start := time.Now()
	errListener := NewErrorListener()
	lexer := JS.NewJavaScriptLexer(antlr.NewInputStream(src))
	lexer.RemoveErrorListeners()
	lexer.AddErrorListener(errListener)
	tokenStream := antlr.NewCommonTokenStream(lexer, antlr.TokenDefaultChannel)
	parser := JS.NewJavaScriptParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(errListener)
	parser.SetErrorHandler(antlr.NewDefaultErrorStrategy())
	fmt.Println("time since start to ast before: ", time.Since(start))
	ast := parser.Program().(*JS.ProgramContext)
	if len(errListener.err) > 0 {
		return nil
	}
	fmt.Println("time since start to ast: ", time.Since(start))
	prog = ssa.NewProgram()
	builder := &builder{
		ast:      ast,
		prog:     prog,
		callback: f,
	}
	prog.Build(builder)
	ssa4analyze.NewAnalyzerGroup(prog).Run()
	return prog
}
