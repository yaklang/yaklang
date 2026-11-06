package compiler

import (
	"fmt"
	"strings"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) llvmFunctionName(fn *ssa.Function) string {
	if fn == nil {
		return ""
	}

	// External/runtime functions must keep their symbol names stable.
	if fn.IsExtern() {
		return fn.GetName()
	}

	// YakSSA uses a synthetic "@main" container. Every user-defined
	// top-level function gets a private prefix before entering the LLVM module.
	// Calls are resolved from SSA function objects, so the LLVM symbol does not
	// need to preserve the Yak source name. Keeping all user symbols in their
	// own namespace avoids collisions with libc/libgc/libgcc symbols (e.g.
	// a Yak function named `read` must never replace libc's read()). Anonymous
	// closures nested in @main use ID-based names below.
	parent := fn.GetParent()
	if parent == nil || parent.GetName() == "@main" {
		name := fn.GetName()
		if name == "@main" {
			// renameConflictingMainFunctions handles this synthetic container.
			return name
		}
		if !strings.HasPrefix(name, "@main$") {
			return abi.UserSymbolPrefix + strings.TrimPrefix(name, "@")
		}
	}

	// Nested/anonymous functions can have duplicate human names (e.g. "f$1").
	// Use an ID-based name to ensure a unique LLVM symbol.
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
