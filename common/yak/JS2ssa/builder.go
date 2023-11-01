package js2ssa

import (
	"github.com/antlr4-go/antlr/v4"
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

func ParseSSA(src string, f func(*ssa.FunctionBuilder)) (prog *ssa.Program) {
	defer func() {
		if r := recover(); r != nil {
			// fmt.Println("Recovered in parseSSA", r)
			// debug.PrintStack()
			prog = nil
		}
	}()

	inputStream := antlr.NewInputStream(src)
	lex := JS.NewJavaScriptLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	ast := JS.NewJavaScriptParser(tokenStream).Program().(*JS.ProgramContext)
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
