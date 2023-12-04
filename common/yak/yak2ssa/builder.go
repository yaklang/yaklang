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

func ParseSSA(src string, f func(*ssa.FunctionBuilder)) (prog *ssa.Program) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("recover from yak2ssa.ParseSSA: ", r)
			// fmt.Println("\n\n\n!!!!!!!\n\n!!!!!\n\nRecovered in parseSSA", r)
			// debug.PrintStack()
			prog = nil
		}
	}()

	inputStream := antlr.NewInputStream(src)
	lex := yak.NewYaklangLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	ast := yak.NewYaklangParser(tokenStream).Program().(*yak.ProgramContext)
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
