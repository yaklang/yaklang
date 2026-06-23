package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

type LLVMExternType uint8

const (
	ExternTypeVoid LLVMExternType = iota
	ExternTypeI64
	ExternTypePtr
)

type ExternBinding struct {
	// Symbol is the target runtime symbol for a direct extern call.
	// Symbol bindings are always invoked through the unified InvokeContext ABI
	// (`void(i8* ctx)`). `Params` are therefore not marshalled at call sites;
	// they are only kept for legacy configuration detection and should remain
	// empty on supported bindings.
	// When DispatchID is non-zero, Symbol/Params/Return are ignored and the call
	// is routed through the builtin dispatcher.
	Symbol string
	Params []LLVMExternType
	Return LLVMExternType

	// DispatchID identifies a stdlib/runtime builtin that should be invoked via
	// the runtime-owned dispatch table. Keep it opaque to reduce the number of
	// exported symbols in the final binary.
	DispatchID abi.FuncID
}

// defaultExternBindings lists runtime builtins that stay in the minimal runtime
// instead of yaklib registration. Everything else (os, sync, yakit, poc, ...)
// is resolved via compile-time yaklib dependency collection.
var defaultExternBindings = map[string]ExternBinding{
	"println": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDPrintln,
	},
	"print": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDPrint,
	},
	"printf": {
		Return:     ExternTypeVoid,
		DispatchID: abi.IDPrintf,
	},
	"append": {
		Return:     ExternTypePtr,
		DispatchID: abi.IDAppend,
	},
}

func cloneExternBindings(src map[string]ExternBinding) map[string]ExternBinding {
	out := make(map[string]ExternBinding, len(src))
	for k, v := range src {
		params := make([]LLVMExternType, len(v.Params))
		copy(params, v.Params)
		out[k] = ExternBinding{
			Symbol:     v.Symbol,
			Params:     params,
			Return:     v.Return,
			DispatchID: v.DispatchID,
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
			Symbol:     v.Symbol,
			Params:     params,
			Return:     v.Return,
			DispatchID: v.DispatchID,
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

func validateExternBindingCallABI(name string, binding ExternBinding) error {
	if binding.DispatchID != 0 || binding.Symbol == "" {
		return nil
	}
	if len(binding.Params) == 0 {
		return nil
	}
	return fmt.Errorf("extern binding %q uses legacy parameter ABI; symbol bindings are invoked via InvokeContext and must leave Params empty", name)
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
