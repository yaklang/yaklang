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

	gcName := c.runtimeSymName(abi.RuntimeGCSymbol)
	gcFn := mod.NamedFunction(gcName)
	if gcFn.IsNil() {
		gcType := llvm.FunctionType(c.LLVMCtx.VoidType(), nil, false)
		gcFn = llvm.AddFunction(mod, gcName, gcType)
	}
	var printFn llvm.Value
	var printType llvm.Type
	if printEntryResult {
		printName := c.runtimeSymName(abi.InternalPrintIntSymbol)
		printFn = mod.NamedFunction(printName)
		printType = llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{c.LLVMCtx.Int64Type()}, false)
		if printFn.IsNil() {
			printFn = llvm.AddFunction(mod, printName, printType)
		}
	}

	mainType := llvm.FunctionType(c.LLVMCtx.Int32Type(), nil, false)
	mainFn := llvm.AddFunction(mod, "main", mainType)
	entryBB := c.LLVMCtx.AddBasicBlock(mainFn, "entry")
	c.Builder.SetInsertPointAtEnd(entryBB)

	// Register global builtins (yak_register_globals)
	c.emitModuleRegistrationCall("yak_register_globals")

	// Register each used yaklib module (yak_register_module_<m>)
	for module := range c.yaklibDeps {
		if module == "" {
			continue // global builtins, handled by yak_register_globals
		}
		c.emitModuleRegistrationCall("yak_register_module_" + module)
	}

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

	// Deliberately do NOT wait for outstanding `go func` goroutines here.
	//
	// The yak VM returns from a script as soon as its main flow ends and leaves
	// background goroutines running; waitAllAsyncCallFinish() exists precisely
	// because that wait is not implicit. Waiting unconditionally deadlocks every
	// script that leaves a long-lived goroutine behind — the common "start a
	// server, drive it, return" shape never finishes its serve loop, so the
	// process hung forever instead of exiting like the VM does.
	c.Builder.CreateCall(gcFn.GlobalValueType(), gcFn, nil, "")

	// Propagate an unhandled top-level panic (e.g. a failed assert or an
	// explicit panic()) to a non-zero process exit code. The runtime stores the
	// panic value in the invoke context's WordPanic slot and logs it to stderr;
	// without this check a failing script would exit 0, masking real failures.
	i64 := c.LLVMCtx.Int64Type()
	panicSlot, err := c.loadCtxWordFrom(ctxI64, abi.WordPanic, "yak_entry_panic")
	if err != nil {
		return err
	}
	hasPanic := c.Builder.CreateICmp(llvm.IntNE, panicSlot, llvm.ConstInt(i64, 0, false), "yak_has_panic")

	// Normal exit block: return the entry function's return value truncated to i32.
	normalBB := c.LLVMCtx.AddBasicBlock(mainFn, "yak_exit_normal")
	c.Builder.SetInsertPointAtEnd(normalBB)
	normalExit := c.Builder.CreateTrunc(ret, c.LLVMCtx.Int32Type(), "exit_code")
	c.Builder.CreateRet(normalExit)

	// Panic exit block: return a non-zero code (255) so a failing script
	// (assert / panic) is detectable by the caller / test harness.
	// Assert failures only store the message on the context; print it before
	// exiting so a non-zero status is not an empty capture.
	panicBB := c.LLVMCtx.AddBasicBlock(mainFn, "yak_exit_panic")
	c.Builder.SetInsertPointAtEnd(panicBB)
	// die(err) stores the error shadow. to_cstring only understands string
	// shadows and C strings, so an error object is printed as the shadow
	// header magic ("Q2ASSKAY"). to_string turns errors and other values
	// into a string shadow first.
	toStrFn, toStrType := c.getOrInsertRuntimeToString()
	panicStr := c.Builder.CreateCall(toStrType, toStrFn, []llvm.Value{panicSlot}, "yak_panic_str")
	cstrFn, cstrType := c.getOrInsertRuntimeToCString()
	panicMsg := c.Builder.CreateCall(cstrType, cstrFn, []llvm.Value{panicStr}, "yak_panic_cstr")
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	putsType := llvm.FunctionType(c.LLVMCtx.Int32Type(), []llvm.Type{i8Ptr}, false)
	putsFn := mod.NamedFunction("puts")
	if putsFn.IsNil() {
		putsFn = llvm.AddFunction(mod, "puts", putsType)
	}
	c.Builder.CreateCall(putsType, putsFn, []llvm.Value{panicMsg}, "")
	c.Builder.CreateRet(llvm.ConstInt(c.LLVMCtx.Int32Type(), 255, false))

	// Branch from the entry block to panic/normal based on the panic slot.
	c.Builder.SetInsertPointAtEnd(entryBB)
	c.Builder.CreateCondBr(hasPanic, panicBB, normalBB)
	return nil
}

// emitModuleRegistrationCall declares and calls a C-exported registration
// function (e.g. yak_register_module_poc, yak_register_globals) in the
// libyak.a runtime. The call creates a link-time reference that prevents
// lld --gc-sections from dropping the corresponding .text.mod_<module> section.
func (c *Compiler) emitModuleRegistrationCall(symbol string) {
	if c == nil || symbol == "" {
		return
	}
	fn := c.Mod.NamedFunction(symbol)
	if fn.IsNil() {
		fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), nil, false)
		fn = llvm.AddFunction(c.Mod, symbol, fnType)
	}
	c.Builder.CreateCall(fn.GlobalValueType(), fn, nil, "")
}
