package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/dispatch"
)

func (c *Compiler) getOrInsertStdlibDispatcher() (llvm.Value, llvm.Type) {
	fn := c.Mod.NamedFunction(dispatch.DispatcherSymbol)

	i64 := c.LLVMCtx.Int64Type()
	argvPtr := llvm.PointerType(i64, 0)
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64, argvPtr}, false)

	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, dispatch.DispatcherSymbol, fnType)
	}
	return fn, fnType
}

func (c *Compiler) compileStdlibDispatchCall(inst *ssa.Call, binding ExternBinding) error {
	dispatcher, dispatcherType := c.getOrInsertStdlibDispatcher()

	argc := len(inst.Args)
	i64 := c.LLVMCtx.Int64Type()
	argvPtr := llvm.ConstPointerNull(llvm.PointerType(i64, 0))
	const yakTaggedPointerMask uint64 = 1 << 62
	tagPointers := shouldTagStdlibArgPointers(binding.DispatchID)

	if argc > 0 {
		mallocFn, mallocType := c.getOrInsertMalloc()
		sizeBytes := llvm.ConstInt(i64, uint64(argc*8), false)
		rawPtr := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{sizeBytes}, "yak_std_argv_mem")
		argvPtr = c.Builder.CreateIntToPtr(rawPtr, llvm.PointerType(i64, 0), "yak_std_argv_ptr")

		for i, argID := range inst.Args {
			argVal, err := c.getValue(inst, argID)
			if err != nil {
				return fmt.Errorf("compileStdlibDispatchCall: failed to resolve argument %d: %w", i, err)
			}
			isPointer := argVal.Type().IntTypeWidth() == 0
			argI64 := c.coerceToInt64(argVal)
			if tagPointers && isPointer {
				tag := llvm.ConstInt(c.LLVMCtx.Int64Type(), yakTaggedPointerMask, false)
				argI64 = buildOr(c.Builder, argI64, tag, "yak_std_arg_tag")
			}
			idx := llvm.ConstInt(i64, uint64(i), false)
			elemPtr := c.Builder.CreateGEP(i64, argvPtr, []llvm.Value{idx}, "")
			c.Builder.CreateStore(argI64, elemPtr)
		}
	}

	idVal := llvm.ConstInt(i64, uint64(binding.DispatchID), false)
	argcVal := llvm.ConstInt(i64, uint64(argc), false)
	callResult := c.Builder.CreateCall(dispatcherType, dispatcher, []llvm.Value{idVal, argcVal, argvPtr}, "")

	switch binding.Return {
	case ExternTypePtr:
		callResult = c.Builder.CreateIntToPtr(callResult, llvm.PointerType(c.LLVMCtx.Int8Type(), 0), "yak_std_ret_ptr")
	case ExternTypeVoid:
		// keep as-is; return is ignored by most callers anyway
	}

	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = callResult
		if err := c.maybeEmitMemberSet(inst, inst, callResult); err != nil {
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
