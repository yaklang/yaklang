package antlr4yak

import (
	"context"
	"fmt"

	"github.com/yaklang/yaklang/common/yak/antlr4util"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakast"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

var buildinLib = make(map[string]interface{})

type FakeCompiler struct {
	code   string
	engine *Engine
}

func Import(name string, f interface{}) {
	buildinLib[name] = f
}

func (f *FakeCompiler) GetErrors() antlr4util.SourceCodeErrors {
	return compiler(f.code).GetErrors()
}

func (f *FakeCompiler) GetFormattedCode() string {
	return compiler(f.code).GetFormattedCode()
}

type FakeVM struct {
	code             string
	engine           *Engine
	executorCompiler *yakast.YakCompiler
}

func (f *FakeVM) Exec() {
	f.DebugExec()
}

func (f *FakeVM) NormalExec() {
	err := f.engine.SafeEval(context.Background(), f.code)
	if err != nil {
		panic(fmt.Sprintf("\n==============\n%s\n==============\n", err.Error()))
	}
}

func (f *FakeVM) SafeExec() error {
	return f.engine.SafeEval(context.Background(), f.code)
}

func (f *FakeVM) DebugExec() {
	f.engine.EnableDebug()
	err := f.engine.SafeEval(context.Background(), f.code)
	if err != nil {
		panic(fmt.Sprintf("\n==============\n%s\n==============\n", err.Error()))
	}
}

func (f *FakeVM) GetCodes() []*yakvm.Code {
	return compiler(f.code).GetOpcodes()
}

type Executor struct {
	Compiler *FakeCompiler
	VM       *FakeVM
}

func compiler(code string) *yakast.YakCompiler {
	vt := yakast.NewYakCompiler()
	vt.Compiler(code)
	// inputStream := antlr.NewInputStream(code)
	// lex := yak.NewYaklangLexer(inputStream)
	// lex.RemoveErrorListeners()
	// lex.AddErrorListener(vt.GetLexerErrorListener())
	// tokenStream := antlr.NewCommonTokenStream(lex, antlr.TokenDefaultChannel)
	// p := yak.NewYaklangParser(tokenStream)
	// vt.AntlrTokenStream = tokenStream
	// p.AddErrorListener(vt.GetParserErrorListener())
	// p.AddErrorListener(vt.GetParserErrorListener())
	// vt.VisitProgram(p.Program().(*yak.ProgramContext))
	return vt
}

func NewExecutor(i string) *Executor {
	e := New()
	e.ImportLibs(buildinLib)
	return &Executor{
		Compiler: &FakeCompiler{
			code: i,
		},
		VM: &FakeVM{
			code:   i,
			engine: e,
		},
	}
}
