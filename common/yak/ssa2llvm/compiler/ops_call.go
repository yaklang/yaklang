package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) getOrInsertRuntimeSpawn() (llvm.Value, llvm.Type) {
	name := "yak_runtime_spawn"
	fn := c.Mod.NamedFunction(name)

	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrDeclareExternCallable(symbol string) (llvm.Value, llvm.Type) {
	if symbol == "" {
		return llvm.Value{}, llvm.Type{}
	}
	fn := c.Mod.NamedFunction(symbol)

	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, symbol, fnType)
	}
	return fn, fnType
}

func (c *Compiler) compileInvokeCallable(inst *ssa.Call, llvmFn llvm.Value, llvmFnType llvm.Type, argIDs []int64) error {
	if inst == nil {
		return nil
	}
	if llvmFn.IsNil() || llvmFnType.C == nil {
		return fmt.Errorf("compileInvokeCallable: missing llvm function for call %d", inst.GetId())
	}

	argc := len(argIDs)
	ctxI8, ctxI64, err := c.allocInvokeContext(argc, "yak_call_ctx")
	if err != nil {
		return err
	}

	i64 := c.LLVMCtx.Int64Type()
	target := c.Builder.CreatePtrToInt(llvmFn, i64, "yak_call_target")
	if err := c.initInvokeContext(ctxI64, abi.KindCallable, target, argc); err != nil {
		return err
	}

	zero := llvm.ConstInt(i64, 0, false)
	for i, argID := range argIDs {
		argVal, err := c.getValue(inst, argID)
		if err != nil {
			return fmt.Errorf("compileInvokeCallable: failed to resolve argument %d: %w", i, err)
		}
		argI64 := c.coerceToInt64(argVal)
		if err := c.storeInvokeContextArg(ctxI64, i, argI64); err != nil {
			return err
		}
		if err := c.storeInvokeContextRoot(ctxI64, argc, i, zero); err != nil {
			return err
		}
	}

	if inst.Async {
		spawnFn, spawnType := c.getOrInsertRuntimeSpawn()
		c.Builder.CreateCall(spawnType, spawnFn, []llvm.Value{ctxI8}, "")
		if inst.GetId() > 0 {
			c.Values[inst.GetId()] = zero
			if err := c.maybeEmitMemberSet(inst, inst, zero); err != nil {
				return err
			}
		}
		return nil
	}

	c.Builder.CreateCall(llvmFnType, llvmFn, []llvm.Value{ctxI8}, "")
	ret, err := c.loadCtxWordFrom(ctxI64, abi.WordRet, "")
	if err != nil {
		return err
	}
	ret = c.coerceToInt64(ret)

	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = ret
		if err := c.maybeEmitMemberSet(inst, inst, ret); err != nil {
			return err
		}
	}
	return nil
}

// compileCall compiles a ssa.Call instruction to LLVM IR.
func (c *Compiler) compileCall(inst *ssa.Call) error {
	fn := inst.GetFunc()
	var calleeVal ssa.Value
	if fn != nil {
		if v, ok := fn.GetValueById(inst.Method); ok && v != nil {
			if vv, ok := v.(ssa.Value); ok {
				calleeVal = vv
			}
		}
	}

	// YakSSA uses function-typed member values (e.g. object-factor methods) where the
	// call target is an Undefined MemberCall but the FunctionType.This points at the
	// actual SSA function implementation. Prefer ID-based resolution to avoid name
	// collisions (e.g. duplicated "f$1").
	if calleeVal != nil {
		if ssaFn, ok := ssa.ToFunction(calleeVal); ok && ssaFn != nil && !ssaFn.IsExtern() {
			llvmFn, llvmFnType := c.getOrDeclareLLVMFunction(ssaFn)
			argIDs := append([]int64{}, inst.Args...)
			argIDs = append(argIDs, inst.ArgMember...)
			return c.compileInvokeCallable(inst, llvmFn, llvmFnType, argIDs)
		}
		if calleeVal.IsMember() {
			if ft, ok := calleeVal.GetType().(*ssa.FunctionType); ok && ft != nil && ft.This != nil && !ft.This.IsExtern() {
				llvmFn, llvmFnType := c.getOrDeclareLLVMFunction(ft.This)
				argIDs := append([]int64{}, inst.Args...)
				argIDs = append(argIDs, inst.ArgMember...)
				return c.compileInvokeCallable(inst, llvmFn, llvmFnType, argIDs)
			}
		}
	}

	calleeName := c.resolveCalleeName(fn, inst.Method)

	// Stdlib dispatch calls.
	if binding, ok := c.getExternBinding(calleeName); ok && binding.DispatchID != 0 {
		if inst.Async {
			return c.compileAsyncStdlibDispatchCall(inst, binding)
		}
		return c.compileStdlibDispatchCall(inst, binding)
	}

	// Context-ABI extern/hook calls.
	if binding, ok := c.getExternBinding(calleeName); ok && binding.Symbol != "" {
		llvmFn, llvmFnType := c.getOrDeclareExternCallable(binding.Symbol)
		argIDs := append([]int64{}, inst.Args...)
		return c.compileInvokeCallable(inst, llvmFn, llvmFnType, argIDs)
	}

	// Fallback: call a named function using the context ABI.
	llvmFn, llvmFnType := c.getOrDeclareExternCallable(calleeName)
	if !llvmFn.IsNil() {
		argIDs := append([]int64{}, inst.Args...)
		return c.compileInvokeCallable(inst, llvmFn, llvmFnType, argIDs)
	}

	return fmt.Errorf("compileCall: unable to resolve callee %q", calleeName)
}
