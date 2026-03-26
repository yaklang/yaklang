package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) addMainWrapperToModule(entryFunc string, printEntryResult bool) error {
	if c == nil {
		return fmt.Errorf("addMainWrapperToModule: compiler is nil")
	}
	if entryFunc == "" {
		return fmt.Errorf("addMainWrapperToModule: missing entry function name")
	}

	mod := c.Mod
	if !mod.NamedFunction("main").IsNil() {
		return fmt.Errorf("addMainWrapperToModule: module already defines main()")
	}

	entry := mod.NamedFunction(entryFunc)
	if entry.IsNil() {
		return fmt.Errorf("addMainWrapperToModule: entry function %q not found in module", entryFunc)
	}

	gcFn := mod.NamedFunction("yak_runtime_gc")
	if gcFn.IsNil() {
		gcType := llvm.FunctionType(c.LLVMCtx.VoidType(), nil, false)
		gcFn = llvm.AddFunction(mod, "yak_runtime_gc", gcType)
	}
	waitAsyncFn := mod.NamedFunction("yak_runtime_wait_async")
	if waitAsyncFn.IsNil() {
		waitType := llvm.FunctionType(c.LLVMCtx.VoidType(), nil, false)
		waitAsyncFn = llvm.AddFunction(mod, "yak_runtime_wait_async", waitType)
	}

	var printFn llvm.Value
	var printType llvm.Type
	if printEntryResult {
		printFn = mod.NamedFunction("yak_internal_print_int")
		printType = llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{c.LLVMCtx.Int64Type()}, false)
		if printFn.IsNil() {
			printFn = llvm.AddFunction(mod, "yak_internal_print_int", printType)
		}
	}

	mainType := llvm.FunctionType(c.LLVMCtx.Int32Type(), nil, false)
	mainFn := llvm.AddFunction(mod, "main", mainType)
	entryBB := c.LLVMCtx.AddBasicBlock(mainFn, "entry")
	c.Builder.SetInsertPointAtEnd(entryBB)

	ctxI8, ctxI64, err := c.allocInvokeContext(0, "yak_entry_ctx")
	if err != nil {
		return err
	}

	target := c.Builder.CreatePtrToInt(entry, c.LLVMCtx.Int64Type(), "yak_entry_target")
	if err := c.initInvokeContext(ctxI64, abi.KindCallable, target, 0); err != nil {
		return err
	}

	c.emitRuntimeInvoke(ctxI8)
	ret, err := c.loadCtxWordFrom(ctxI64, abi.WordRet, "yak_entry_ret")
	if err != nil {
		return err
	}

	if printEntryResult {
		c.Builder.CreateCall(printType, printFn, []llvm.Value{ret}, "")
	}

	c.Builder.CreateCall(waitAsyncFn.GlobalValueType(), waitAsyncFn, nil, "")
	c.Builder.CreateCall(gcFn.GlobalValueType(), gcFn, nil, "")

	exitCode := c.Builder.CreateTrunc(ret, c.LLVMCtx.Int32Type(), "exit_code")
	c.Builder.CreateRet(exitCode)
	return nil
}
