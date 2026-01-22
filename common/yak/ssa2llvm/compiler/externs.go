package compiler

import (
	"github.com/yaklang/go-llvm"
)

var externs = map[string]string{
	"println": "yak_internal_print_int",
}

func getExternFunction(name string) string {
	if mapped, ok := externs[name]; ok {
		return mapped
	}
	return name
}

func (c *Compiler) ensureExternDeclaration(name string) llvm.Value {
	externName := getExternFunction(name)
	fn := c.Mod.NamedFunction(externName)
	if !fn.IsNil() {
		return fn
	}

	// Create declaration
	// Currently hardcoded for println(int64) -> void
	if name == "println" {
		params := []llvm.Type{c.LLVMCtx.Int64Type()}
		retType := c.LLVMCtx.VoidType()
		fnType := llvm.FunctionType(retType, params, false)
		fn = llvm.AddFunction(c.Mod, externName, fnType)
	}

	return fn
}
