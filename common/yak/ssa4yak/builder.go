package ssa4yak

import (
	"github.com/antlr/antlr4/runtime/Go/antlr/v4"
	yak "github.com/yaklang/yaklang/common/yak/antlr4yak/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

type astbuilder struct {
	*ssa.FunctionBuilder
}

type builder struct {
	ast  *yak.YaklangParser
	prog *ssa.Program
}

// build implements ssa.builder.
func (b *builder) Build() {
	pkg := b.prog.NewPackage("main")
	main := pkg.NewFunction("yak-main")
	funcbuilder := ssa.NewBuilder(main, nil)
	astbuilder := astbuilder{
		FunctionBuilder: funcbuilder,
	}
	astbuilder.build(b.ast)
	astbuilder.Finish()
}

func NewBuilder(ast *yak.YaklangParser, prog *ssa.Program) *builder {
	return &builder{
		ast:  ast,
		prog: prog,
	}
}

var _ (ssa.Builder) = (*builder)(nil)

func ParseSSA(src string, opt ...ssa4analyze.Option) *ssa.Program {
	inputStream := antlr.NewInputStream(src)
	lex := yak.NewYaklangLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	ast := yak.NewYaklangParser(tokenStream)
	prog := ssa.NewProgram()
	builder := &builder{
		ast:  ast,
		prog: prog,
	}
	prog.Build(builder)
	if len(opt) == 0 {
		opt = append(opt, ssa4analyze.WithPass(true))
	}
	ssa4analyze.NewAnalyzerGroup(
		prog,
		opt...,
	).Run()
	return prog
}
