package antlr4Lua

import (
	"context"
	"github.com/yaklang/yaklang/common/yak/antlr4Lua/infrastructure"
	"github.com/yaklang/yaklang/common/yak/antlr4Lua/luaast"
	"github.com/yaklang/yaklang/common/yak/antlr4yak/yakvm"
)

type Engine struct {
	rootSymbol     *yakvm.SymbolTable
	rootLabel      *infrastructure.LabelTable
	libs           map[string]interface{}
	vm             *yakvm.VirtualMachine
	debug          bool
	sourceCode     string
	preIncludeCode string
}

func New() *Engine {
	table := yakvm.NewSymbolTable()
	label := infrastructure.NewLabelTable()
	vm := yakvm.NewWithSymbolTable(table)
	vmConfig := yakvm.NewVMConfig()
	vmConfig.SetYVMMode(yakvm.LUA)
	vm.SetConfig(vmConfig)
	preIncludeCode := `
    function ipairs(a)
    iter = function(a, i)
        i = i + 1
        local v = a[i]
        if v then
            return i, v
        end
    end
    return iter, a, 0
end

function pairs (t)
      return next, t, nil
end
`
	//preIncludeCode = ""
	engine := &Engine{
		rootSymbol:     table,
		rootLabel:      label,
		vm:             vm,
		preIncludeCode: preIncludeCode,
	}
	return engine
}

func (e *Engine) ImportLibs(libs map[string]interface{}) {
	e.vm.ImportLibs(libs)
}

func (e *Engine) Trans(sourceCode string) ([]*yakvm.Code, error) {
	translator, err := e._doTranslate(sourceCode)
	if err != nil {
		return nil, err
	}
	return translator.GetOpcodes(), err
}

func (e *Engine) MustTranslate(sourceCode string) []*yakvm.Code {
	translator, err := e._doTranslate(sourceCode)
	if err != nil {
		panic(err)
	}
	return translator.GetOpcodes()
}

func (e *Engine) _doTranslate(code string) (*luaast.LuaTranslator, error) {
	translator := luaast.NewLuaTranslatorWithTable(e.rootSymbol, e.rootLabel)
	translator.Translate(code)
	if len(translator.GetErrors()) > 0 {
		return nil, translator.GetErrors()
	}
	return translator, nil
}

func (e *Engine) Eval(ctx context.Context, sourceCode string) error {
	e.vm.SetDebug(e.debug)
	concatenateSource := e.preIncludeCode + sourceCode
	flag := yakvm.None
	opCodes := e.MustTranslate(concatenateSource)
	yakvm.ShowOpcodes(opCodes)
	return e.vm.ExecYakCode(ctx, concatenateSource, opCodes, flag)
}
