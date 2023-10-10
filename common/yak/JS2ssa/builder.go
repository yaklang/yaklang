package js2ssa

import (
	"fmt"

	"github.com/antlr4-go/antlr/v4"
	JS "github.com/yaklang/yaklang/common/yak/antlr4JS/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"

)
type astbuilder struct {
	*ssa.FunctionBuilder
}

type builder struct {
	ast *JS.JavaScriptParser
	prog *ssa.Program
	c *config
}

func (b *builder) Build() {
	pkg := ssa.NewPackage("main")
	b.prog.AddPackage(pkg)
	main := pkg.NewFunction("yak-main")
	funcbuilder := ssa.NewBuilder(main, nil)
	funcbuilder.WithExternInstance(b.c.symboltable)

	astbuilder := astbuilder{
		FunctionBuilder: funcbuilder,
	}
	astbuilder.build(b.ast)
	astbuilder.Finish()

}

var _ (ssa.Builder) = (*builder)(nil)

type config struct {
	analyzeOpt 	[]ssa4analyze.Option
	symboltable map[string]any
	typeMethod 	map[string]any
}

func defaultConfig() *config {
	return &config{
		analyzeOpt: make([]ssa4analyze.Option, 0),
		symboltable: nil,
		typeMethod: nil,
	}
}

type Option func (*config)

func WithAnalyzeOpt(opt ...ssa4analyze.Option) Option {
	return func(c *config) {
		c.analyzeOpt = append(c.analyzeOpt, opt...)
	}
}

func WithSymbolTable(table map[string]any) Option {
	return func(c *config) {
		c.symboltable = table
	}
}

func WithTypeMethod(table map[string]any) Option {
	return func(c *config) {
		c.typeMethod = table
	}
}

func parseSSA(src string, opt ...Option) (prog *ssa.Program) {
	defer func() {
		if r := recover(); r != nil {
			fmt.Println("Recovered in parseSSA", r)
			prog = nil
		}
	}()
	
	c := defaultConfig()
	for _, f:= range opt {
		f(c)
	}

	inputStream := antlr.NewInputStream(src)
	lex := JS.NewJavaScriptLexer(inputStream)
	tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	ast := JS.NewJavaScriptParser(tokenStream)
	prog = ssa.NewProgram()
	builder := &builder{
		ast: ast,
		prog: prog,
		c: c,
	}
	prog.Build(builder)
	ssa4analyze.NewAnalyzerGroup(
		prog,
		c.analyzeOpt...,
	).Run()
	return prog
}