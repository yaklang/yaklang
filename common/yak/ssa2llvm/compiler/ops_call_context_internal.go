package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

const yakTaggedPointerMask uint64 = 1 << 62

type contextCallSpec struct {
	inst      *ssa.Call
	kind      uint64
	target    llvm.Value
	args      []contextCallArg
	async     bool
	ctxName   string
	errPrefix string
}

type contextCallArg struct {
	ssaID         int64
	value         llvm.Value
	root          llvm.Value
	tagPointerArg bool
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

	ssaVal, ok := fn.GetValueById(argID)
	if !ok || ssaVal == nil || !c.ssaValueIsPointer(ssaVal, fn) {
		return argI64, root, nil
	}

	root = argI64
	tag := llvm.ConstInt(i64, yakTaggedPointerMask, false)
	argI64 = c.Builder.CreateOr(argI64, tag, "yak_ctx_arg_tag")
	return argI64, root, nil
}

func (c *Compiler) resolveContextCallArgValue(inst *ssa.Call, arg contextCallArg) (llvm.Value, llvm.Value, error) {
	if arg.ssaID > 0 {
		return c.resolveContextCallArg(inst, arg.ssaID, arg.tagPointerArg)
	}

	i64 := c.LLVMCtx.Int64Type()
	value := arg.value
	if value.IsNil() {
		value = llvm.ConstInt(i64, 0, false)
	}
	value = c.coerceToInt64(value)
	root := arg.root
	if root.IsNil() {
		root = llvm.ConstInt(i64, 0, false)
	} else {
		root = c.coerceToInt64(root)
	}
	return value, root, nil
}

func (c *Compiler) emitContextCall(spec contextCallSpec) (llvm.Value, error) {
	if spec.inst == nil {
		return llvm.Value{}, fmt.Errorf("emitContextCall: missing call instruction")
	}
	if spec.target.IsNil() {
		return llvm.Value{}, fmt.Errorf("emitContextCall: missing target for call %d", spec.inst.GetId())
	}

	argc := len(spec.args)
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

	for index, arg := range spec.args {
		argI64, root, err := c.resolveContextCallArgValue(spec.inst, arg)
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

func shouldTagStdlibArgPointers(id abi.FuncID) bool {
	switch id {
	case abi.IDPrint, abi.IDPrintf, abi.IDPrintln,
		abi.IDAppend,
		abi.IDYakitInfo, abi.IDYakitWarn, abi.IDYakitDebug, abi.IDYakitError:
		return true
	default:
		return false
	}
}

func ssaArgs(argIDs []int64, tagPointerArgs bool) []contextCallArg {
	args := make([]contextCallArg, 0, len(argIDs))
	for _, argID := range argIDs {
		args = append(args, contextCallArg{
			ssaID:         argID,
			tagPointerArg: tagPointerArgs,
		})
	}
	return args
}
