package js2ssa

import (
	"fmt"
	"runtime/debug"

	"github.com/antlr4-go/antlr/v4"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
)

type astbuilder struct {
	*ssa.FunctionBuilder
}

type builder struct {
	ast  *JS.JavaScriptParser
	prog *ssa.Program
	c    *config
}

func (b *builder) Build() {
	pkg := ssa.NewPackage("main")
	b.prog.AddPackage(pkg)
	main := pkg.NewFunction("yak-main")
	funcbuilder := ssa.NewBuilder(main, nil)
	funcbuilder.WithExternValue(b.c.externValue)
	funcbuilder.WithExternLib(b.c.externLib)

	astbuilder := astbuilder{
		FunctionBuilder: funcbuilder,
	}
	astbuilder.build(b.ast)
	astbuilder.Finish()

}

var _ (ssa.Builder) = (*builder)(nil)

type config struct {
	analyzeOpt []ssa4analyze.Option
	typeMethod map[string]any

	externValue map[string]any
	externLib   map[string]map[string]any
}

func defaultConfig() *config {
	return &config{
		analyzeOpt:  make([]ssa4analyze.Option, 0),
		typeMethod:  nil,
		externValue: nil,
		externLib:   make(map[string]map[string]any),
	}
}

type Option func(*config)

func WithAnalyzeOpt(opt ...ssa4analyze.Option) Option {
	return func(c *config) {
		c.analyzeOpt = append(c.analyzeOpt, opt...)
	}
}

func WithExternValue(table map[string]any) Option {
	return func(c *config) {
		c.externValue = table
	}
}

func WithExternLib(libName string, table map[string]any) Option {
	return func(c *config) {
		c.externLib[libName] = table
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
			debug.PrintStack()
			prog = nil
		}
	}()

	c := defaultConfig()
	for _, f := range opt {
		f(c)
	}

	inputStream := antlr.NewInputStream(src)
	lex := JS.NewJavaScriptLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	ast := JS.NewJavaScriptParser(tokenStream)
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
