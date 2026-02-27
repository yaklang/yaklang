package compiler

import (
	"github.com/yaklang/go-llvm"
)

type LLVMExternType uint8

const (
	ExternTypeVoid LLVMExternType = iota
	ExternTypeI64
	ExternTypePtr
)

type ExternBinding struct {
	Symbol string
	Params []LLVMExternType
	Return LLVMExternType
}

var defaultExternBindings = map[string]ExternBinding{
	"println": {
		Symbol: "yak_internal_print_int",
		Params: []LLVMExternType{ExternTypeI64},
		Return: ExternTypeVoid,
	},
}

func cloneExternBindings(src map[string]ExternBinding) map[string]ExternBinding {
	out := make(map[string]ExternBinding, len(src))
	for k, v := range src {
		params := make([]LLVMExternType, len(v.Params))
		copy(params, v.Params)
		out[k] = ExternBinding{
			Symbol: v.Symbol,
			Params: params,
			Return: v.Return,
		}
	}
	return out
}

func mergeExternBindings(base map[string]ExternBinding, custom map[string]ExternBinding) map[string]ExternBinding {
	out := cloneExternBindings(base)
	for k, v := range custom {
		params := make([]LLVMExternType, len(v.Params))
		copy(params, v.Params)
		out[k] = ExternBinding{
			Symbol: v.Symbol,
			Params: params,
			Return: v.Return,
		}
	}
	return out
}

func (c *Compiler) getExternBinding(name string) (ExternBinding, bool) {
	if c == nil || c.ExternBindings == nil {
		return ExternBinding{}, false
	}
	b, ok := c.ExternBindings[name]
	return b, ok
}

func (c *Compiler) llvmTypeForExtern(t LLVMExternType) llvm.Type {
	switch t {
	case ExternTypeVoid:
		return c.LLVMCtx.VoidType()
	case ExternTypePtr:
		return llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	case ExternTypeI64:
		fallthrough
	default:
		return c.LLVMCtx.Int64Type()
	}
}

func (c *Compiler) ensureExternDeclaration(name string) llvm.Value {
	binding, ok := c.getExternBinding(name)
	if !ok {
		return llvm.Value{}
	}

	fn := c.Mod.NamedFunction(binding.Symbol)
	if !fn.IsNil() {
		return fn
	}

	params := make([]llvm.Type, 0, len(binding.Params))
	for _, p := range binding.Params {
		params = append(params, c.llvmTypeForExtern(p))
	}
	retType := c.llvmTypeForExtern(binding.Return)
	fnType := llvm.FunctionType(retType, params, false)
	fn = llvm.AddFunction(c.Mod, binding.Symbol, fnType)
	return fn
}
