package java2ssa

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/antlr4util"
	javaparser "github.com/yaklang/yaklang/common/yak/java/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type builder struct {
	*ssa.FunctionBuilder

	ast  javaparser.ICompilationUnitContext
	prog *ssa.Program
}

func ParserSSA(src string) *ssa.Program {
	lex := javaparser.NewJavaLexer(antlr.NewInputStream(src))
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	parser := javaparser.NewJavaParser(tokenStream)
	parser.RemoveErrorListeners()
	parser.AddErrorListener(antlr4util.NewLegacyErrorListener())
	prog := ssa.NewProgram()
	build := &builder{
		prog: prog, ast: parser.CompilationUnit(),
	}
	build.Build()
	for _, r := range build.prog.GetErrors() {
		log.Errorf("ssa-ir programe error: %s", r)
	}
	return build.prog
}

func (b *builder) Build() {
	pkg := ssa.NewPackage("main")
	b.prog.AddPackage(pkg)
	main := pkg.NewFunction("main")
	b.FunctionBuilder = ssa.NewBuilder(main, nil)
	b.VisitCompilationUnit(b.ast)
	b.Finish()
}
