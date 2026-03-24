package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/dispatch"
)

const yakTaggedPointerMask uint64 = 1 << 62

type contextCallSpec struct {
	inst           *ssa.Call
	kind           uint64
	target         llvm.Value
	argIDs         []int64
	async          bool
	tagPointerArgs bool
	ctxName        string
	errPrefix      string
}

func (c *Compiler) getOrInsertRuntimeInvoke() (llvm.Value, llvm.Type) {
	fn := c.Mod.NamedFunction(abi.InvokeSymbol)
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(c.LLVMCtx.VoidType(), []llvm.Type{i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, abi.InvokeSymbol, fnType)
	}
	return fn, fnType
}

func (c *Compiler) emitRuntimeInvoke(ctxI8 llvm.Value) {
	invokeFn, invokeType := c.getOrInsertRuntimeInvoke()
	c.Builder.CreateCall(invokeType, invokeFn, []llvm.Value{ctxI8}, "")
}

func (c *Compiler) resolveContextCallArg(inst *ssa.Call, argID int64, tagPointerArgs bool) (llvm.Value, llvm.Value, error) {
	argVal, err := c.getValue(inst, argID)
	if err != nil {
		return llvm.Value{}, llvm.Value{}, err
	}

	i64 := c.LLVMCtx.Int64Type()
	argI64 := c.coerceToInt64(argVal)
	root := llvm.ConstInt(i64, 0, false)
	if !tagPointerArgs || inst == nil {
		return argI64, root, nil
	}

	fn := inst.GetFunc()
	if fn == nil {
		return argI64, root, nil
	}

	ssaValAny, ok := fn.GetValueById(argID)
	if !ok || ssaValAny == nil {
		return argI64, root, nil
	}
	ssaVal, ok := ssaValAny.(ssa.Value)
	if !ok || !c.ssaValueIsPointer(ssaVal, fn) {
		return argI64, root, nil
	}

	root = argI64
	tag := llvm.ConstInt(i64, yakTaggedPointerMask, false)
	argI64 = buildOr(c.Builder, argI64, tag, "yak_ctx_arg_tag")
	return argI64, root, nil
}

func (c *Compiler) emitContextCall(spec contextCallSpec) (llvm.Value, error) {
	if spec.inst == nil {
		return llvm.Value{}, fmt.Errorf("emitContextCall: missing call instruction")
	}
	if spec.target.IsNil() {
		return llvm.Value{}, fmt.Errorf("emitContextCall: missing target for call %d", spec.inst.GetId())
	}

	argc := len(spec.argIDs)
	ctxName := spec.ctxName
	if ctxName == "" {
		ctxName = "yak_call_ctx"
	}

	ctxI8, ctxI64, err := c.allocInvokeContext(argc, ctxName)
	if err != nil {
		return llvm.Value{}, err
	}
	if err := c.initInvokeContext(ctxI64, spec.kind, spec.target, argc); err != nil {
		return llvm.Value{}, err
	}
	if spec.async {
		i64 := c.LLVMCtx.Int64Type()
		if err := c.storeCtxWordFrom(ctxI64, abi.WordFlags, llvm.ConstInt(i64, abi.FlagAsync, false)); err != nil {
			return llvm.Value{}, err
		}
	}

	for index, argID := range spec.argIDs {
		argI64, root, err := c.resolveContextCallArg(spec.inst, argID, spec.tagPointerArgs)
		if err != nil {
			prefix := spec.errPrefix
			if prefix == "" {
				prefix = "emitContextCall"
			}
			return llvm.Value{}, fmt.Errorf("%s: failed to resolve argument %d: %w", prefix, index, err)
		}
		if err := c.storeInvokeContextArg(ctxI64, index, argI64); err != nil {
			return llvm.Value{}, err
		}
		if err := c.storeInvokeContextRoot(ctxI64, argc, index, root); err != nil {
			return llvm.Value{}, err
		}
	}

	c.emitRuntimeInvoke(ctxI8)

	zero := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
	if spec.async {
		return zero, nil
	}

	ret, err := c.loadCtxWordFrom(ctxI64, abi.WordRet, "")
	if err != nil {
		return llvm.Value{}, err
	}
	return c.coerceToInt64(ret), nil
}

func (c *Compiler) finishContextCall(inst *ssa.Call, result llvm.Value) error {
	if inst == nil || inst.GetId() <= 0 {
		return nil
	}
	result = c.coerceToInt64(result)
	c.Values[inst.GetId()] = result
	return c.maybeEmitMemberSet(inst, inst, result)
}

func (c *Compiler) lowerResolvedContextCall(spec contextCallSpec) error {
	result, err := c.emitContextCall(spec)
	if err != nil {
		return err
	}
	return c.finishContextCall(spec.inst, result)
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
