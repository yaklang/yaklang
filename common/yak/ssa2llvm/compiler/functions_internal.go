package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) llvmFunctionName(fn *ssa.Function) string {
	if fn == nil {
		return ""
	}

	// External/runtime functions must keep their symbol names stable.
	if fn.IsExtern() {
		return fn.GetName()
	}

	// YakSSA uses a synthetic "@main" container; functions declared at the
	// top-level typically have parent "@main". Keep those names stable so
	// entry resolution (e.g. "check") continues to work.
	parent := fn.GetParent()
	if parent == nil || parent.GetName() == "@main" {
		return fn.GetName()
	}

	// Nested/anonymous functions can have duplicate human names (e.g. "f$1").
	// Use an ID-based name to ensure a stable unique LLVM symbol.
	return fmt.Sprintf("yak_fn_%d", fn.GetId())
}

func (c *Compiler) getOrDeclareLLVMFunction(fn *ssa.Function) (llvm.Value, llvm.Type) {
	if fn == nil {
		return llvm.Value{}, llvm.Type{}
	}
	if existing, ok := c.Funcs[fn.GetId()]; ok && !existing.IsNil() {
		return existing, existing.GlobalValueType()
	}

	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i8Ptr}, false)

	name := c.llvmFunctionName(fn)
	llvmFn := c.Mod.NamedFunction(name)
	if llvmFn.IsNil() {
		llvmFn = llvm.AddFunction(c.Mod, name, fnType)
	}

	c.Funcs[fn.GetId()] = llvmFn
	return llvmFn, fnType
}
