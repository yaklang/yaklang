package compiler

import (
	"fmt"
	"strconv"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) callableContextArgs(inst *ssa.Call, calleeFn *ssa.Function) []contextCallArg {
	argIDs := callframe.BuildCallFrameArgIDs(c.Program, inst, calleeFn)
	if inst != nil && inst.Async {
	}
	args := make([]contextCallArg, 0, len(argIDs))
	for _, argID := range argIDs {
		args = append(args, contextCallArg{
			ssaID:         argID,
			tagPointerArg: c.shouldTagDirectCallArg(inst, argID),
		})
	}
	// Variadic callee: pack the trailing call arguments into a slice for the
	// variadic parameter (sum(1,2,3) -> a = [1,2,3]). The SSA front end keeps
	// the call arguments flat, so the packing happens here at the call site.
	if inst != nil && calleeFn != nil && c.calleeIsVariadic(calleeFn) {
		variadicIndex := len(calleeFn.Params) - 1
		if variadicIndex >= 0 && variadicIndex < len(args) && len(inst.Args) > variadicIndex {
			packed := c.emitVariadicPack(inst, inst.Args[variadicIndex:])
			args[variadicIndex] = contextCallArg{value: packed, tagPointerArg: true}
		}
	}
	return args
}

// calleeIsVariadic reports whether the callee's function type declares a
// variadic (ellipsis) parameter.
func (c *Compiler) calleeIsVariadic(calleeFn *ssa.Function) bool {
	if calleeFn == nil || calleeFn.GetType() == nil {
		return false
	}
	if fnType, ok := calleeFn.GetType().(*ssa.FunctionType); ok && fnType != nil {
		return fnType.IsVariadic
	}
	return false
}

// emitVariadicPack builds a slice shadow holding the given argument values
// and returns its word, for variadic callee calls (sum(1,2,3) -> a=[1,2,3]).
func (c *Compiler) emitVariadicPack(inst *ssa.Call, argIDs []int64) llvm.Value {
	i64 := c.LLVMCtx.Int64Type()
	makeFn, makeType := c.getOrInsertRuntimeMakeSlice()
	length := llvm.ConstInt(i64, uint64(len(argIDs)), false)
	slice := c.Builder.CreateCall(makeType, makeFn, []llvm.Value{
		llvm.ConstInt(i64, uint64(abi.SliceElemAny), false), length, length,
	}, "yak_variadic_slice")
	if len(argIDs) == 0 {
		return slice
	}
	setFn, setType := c.getOrInsertRuntimeSetField()
	objPtr := c.coerceToI8Ptr(slice)
	fn := inst.GetFunc()
	for i, argID := range argIDs {
		val, err := c.getValue(inst, argID)
		if err != nil || val.IsNil() {
			val = llvm.ConstInt(i64, 0, false)
		}
		intVal := c.coerceToInt64(val)
		if fn != nil {
			if ssaVal, ok := fn.GetValueById(argID); ok && ssaVal != nil && c.ssaValueIsPointer(ssaVal, ssaVal.GetFunc()) {
				intVal = c.Builder.CreateOr(intVal, llvm.ConstInt(i64, yakTaggedPointerMask, false), "yak_variadic_tag")
			}
		}
		keyPtr := c.Builder.CreateGlobalStringPtr(strconv.Itoa(i), fmt.Sprintf("yak_variadic_key_%d", i))
		c.Builder.CreateCall(setType, setFn, []llvm.Value{objPtr, keyPtr, intVal, llvm.ConstInt(i64, 0, false)}, "")
	}
	return slice
}

func (c *Compiler) shouldTagDirectCallArg(inst *ssa.Call, argID int64) bool {
	if inst == nil || argID <= 0 {
		return false
	}
	fn := inst.GetFunc()
	if fn == nil {
		return false
	}
	value, ok := fn.GetValueById(argID)
	if !ok || value == nil || value.GetType() == nil {
		return false
	}
	return value.GetType().GetTypeKind() == ssa.StringTypeKind
}
