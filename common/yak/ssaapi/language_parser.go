package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils"
	js2ssa "github.com/yaklang/yaklang/common/yak/JS2ssa"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa4analyze"
	"github.com/yaklang/yaklang/common/yak/yak2ssa"
)

type Language string

const (
	Yak Language = "yak"
	JS  Language = "js"
)

type Build func(string, bool, *ssa.FunctionBuilder) error

var (
	LanguageBuilders = map[Language]Build{
		Yak: yak2ssa.Build,
		JS:  js2ssa.Build,
	}
)

func parse(c *config, prog *ssa.Program) (ret *ssa.Program, err error) {
	defer func() {
		if r := recover(); r != nil {
			//debug.PrintStack()
			ret = nil
			err = utils.Errorf("parse error with panic : %v", r)
		}
	}()

	if prog == nil {
		prog = ssa.NewProgram()
	}

	builder := prog.GetAndCreateMainFunctionBuilder()
	builder.WithExternLib(c.externLib)
	builder.WithExternValue(c.externValue)
	builder.WithExternMethod(c.externMethod)
	builder.WithDefineFunction(c.defineFunc)

	if err := c.Build(c.code, c.ignoreSyntaxErr, builder); err != nil {
		return nil, err
	}

	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
	return prog, nil
}

func feed(c *config, prog *ssa.Program, code string) {
	builder := prog.GetAndCreateMainFunctionBuilder()
	if err := c.Build(code, c.ignoreSyntaxErr, builder); err != nil {
		return
	}
	builder.Finish()
	ssa4analyze.RunAnalyzer(prog)
}
