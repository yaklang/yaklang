package ssa4yak

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
	ast  *yak.YaklangParser
	prog *ssa.Program
	// symbolTable map[string]any
	c *config
}

// build implements ssa.builder.
func (b *builder) Build() {
	pkg := ssa.NewPackage("main")
	b.prog.AddPackage(pkg)
	main := pkg.NewFunction("yak-main")
	funcBuilder := ssa.NewBuilder(main, nil)
	funcBuilder.WithExternInstance(b.c.symbolTable)

	astbuilder := astbuilder{
		FunctionBuilder: funcBuilder,
	}
	astbuilder.build(b.ast)
	astbuilder.Finish()
}

var _ (ssa.Builder) = (*builder)(nil)

type config struct {
	analyzeOpt  []ssa4analyze.Option
	symbolTable map[string]any
	typeMethod  map[string]any
}

func defaultConfig() *config {
	return &config{
		analyzeOpt:  make([]ssa4analyze.Option, 0),
		symbolTable: nil,
		typeMethod:  nil,
	}

}

type Option func(*config)

func WithAnalyzeOpt(opt ...ssa4analyze.Option) Option {
	return func(c *config) {
		c.analyzeOpt = append(c.analyzeOpt, opt...)
	}
}

func WithSymbolTable(table map[string]any) Option {
	return func(c *config) {
		c.symbolTable = table
	}
}

func WithTypeMethod(table map[string]any) Option {
	return func(c *config) {
		c.typeMethod = table
	}
}

func ParseSSA(src string, opt ...Option) (prog *ssa.Program) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in parseSSA", r)
			// fmt.Println(src)
			// debug.PrintStack()
			prog = nil
		}
	}()

	c := defaultConfig()
	for _, f := range opt {
		f(c)
	}

	inputStream := antlr.NewInputStream(src)
	lex := yak.NewYaklangLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	ast := yak.NewYaklangParser(tokenStream)
	prog = ssa.NewProgram()
	builder := &builder{
		ast:  ast,
		prog: prog,
		c:    c,
	}
	prog.Build(builder)
	ssa4analyze.NewAnalyzerGroup(
		prog,
		c.analyzeOpt...,
	).Run()
	return prog
}
