package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/dispatch"
)

func (c *Compiler) getOrInsertStdlibDispatcher() (llvm.Value, llvm.Type) {
	fn := c.Mod.NamedFunction(dispatch.DispatcherSymbol)

	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i8Ptr}, false)

	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, dispatch.DispatcherSymbol, fnType)
	}
	return fn, fnType
}

func (c *Compiler) compileStdlibDispatchCall(inst *ssa.Call, binding ExternBinding) error {
	dispatcher, dispatcherType := c.getOrInsertStdlibDispatcher()

	argc := len(inst.Args)
	i64 := c.LLVMCtx.Int64Type()
	const yakTaggedPointerMask uint64 = 1 << 62
	tagPointers := shouldTagStdlibArgPointers(binding.DispatchID)

	ctxI8, ctxI64, err := c.allocInvokeContext(argc, "yak_std_ctx")
	if err != nil {
		return err
	}
	target := llvm.ConstInt(i64, uint64(binding.DispatchID), false)
	if err := c.initInvokeContext(ctxI64, abi.KindDispatch, target, argc); err != nil {
		return err
	}

	fn := inst.GetFunc()
	for i, argID := range inst.Args {
		argVal, err := c.getValue(inst, argID)
		if err != nil {
			return fmt.Errorf("compileStdlibDispatchCall: failed to resolve argument %d: %w", i, err)
		}

		argI64 := c.coerceToInt64(argVal)
		root := llvm.ConstInt(i64, 0, false)

		isPointer := false
		if fn != nil {
			if ssaValAny, ok := fn.GetValueById(argID); ok && ssaValAny != nil {
				if ssaVal, ok := ssaValAny.(ssa.Value); ok {
					isPointer = c.ssaValueIsPointer(ssaVal, fn)
				}
			}
		}

		if tagPointers && isPointer {
			root = argI64
			tag := llvm.ConstInt(i64, yakTaggedPointerMask, false)
			argI64 = buildOr(c.Builder, argI64, tag, "yak_std_arg_tag")
		}

		if err := c.storeInvokeContextArg(ctxI64, i, argI64); err != nil {
			return err
		}
		if err := c.storeInvokeContextRoot(ctxI64, argc, i, root); err != nil {
			return err
		}
	}

	c.Builder.CreateCall(dispatcherType, dispatcher, []llvm.Value{ctxI8}, "")
	callResult, err := c.loadCtxWordFrom(ctxI64, abi.WordRet, "")
	if err != nil {
		return err
	}

	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = callResult
		if err := c.maybeEmitMemberSet(inst, inst, callResult); err != nil {
			return err
		}
	}

	return nil
}

func (c *Compiler) compileAsyncStdlibDispatchCall(inst *ssa.Call, binding ExternBinding) error {
	argc := len(inst.Args)
	i64 := c.LLVMCtx.Int64Type()
	const yakTaggedPointerMask uint64 = 1 << 62
	tagPointers := shouldTagStdlibArgPointers(binding.DispatchID)

	ctxI8, ctxI64, err := c.allocInvokeContext(argc, "yak_async_std_ctx")
	if err != nil {
		return err
	}
	target := llvm.ConstInt(i64, uint64(binding.DispatchID), false)
	if err := c.initInvokeContext(ctxI64, abi.KindDispatch, target, argc); err != nil {
		return err
	}

	fn := inst.GetFunc()
	for i, argID := range inst.Args {
		argVal, err := c.getValue(inst, argID)
		if err != nil {
			return fmt.Errorf("compileAsyncStdlibDispatchCall: failed to resolve argument %d: %w", i, err)
		}

		argI64 := c.coerceToInt64(argVal)
		root := llvm.ConstInt(i64, 0, false)

		isPointer := false
		if fn != nil {
			if ssaValAny, ok := fn.GetValueById(argID); ok && ssaValAny != nil {
				if ssaVal, ok := ssaValAny.(ssa.Value); ok {
					isPointer = c.ssaValueIsPointer(ssaVal, fn)
				}
			}
		}

		if tagPointers && isPointer {
			root = argI64
			tag := llvm.ConstInt(i64, yakTaggedPointerMask, false)
			argI64 = buildOr(c.Builder, argI64, tag, "yak_std_arg_tag")
		}

		if err := c.storeInvokeContextArg(ctxI64, i, argI64); err != nil {
			return err
		}
		if err := c.storeInvokeContextRoot(ctxI64, argc, i, root); err != nil {
			return err
		}
	}

	spawnFn, spawnType := c.getOrInsertRuntimeSpawn()
	c.Builder.CreateCall(spawnType, spawnFn, []llvm.Value{ctxI8}, "")

	if inst.GetId() > 0 {
		zero := llvm.ConstInt(i64, 0, false)
		c.Values[inst.GetId()] = zero
		if err := c.maybeEmitMemberSet(inst, inst, zero); err != nil {
			return err
		}
	}

	return nil
}

func shouldTagStdlibArgPointers(id dispatch.FuncID) bool {
	switch id {
	case dispatch.IDPrint, dispatch.IDPrintf, dispatch.IDPrintln,
		dispatch.IDYakitInfo, dispatch.IDYakitWarn, dispatch.IDYakitDebug, dispatch.IDYakitError:
		return true
	default:
		return false
	}
}
