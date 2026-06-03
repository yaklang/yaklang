package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) getOrInsertRuntimeMakeCallable() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.MakeCallableSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64, i64, i8Ptr}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) functionValueForArg(fn *ssa.Function, valueID int64) (*ssa.Function, bool) {
	if fn == nil || valueID <= 0 {
		return nil, false
	}
	value, ok := fn.GetValueById(valueID)
	if !ok || value == nil {
		return nil, false
	}
	return c.resolveFunctionValue(value)
}

func (c *Compiler) resolveFunctionValue(value ssa.Value) (*ssa.Function, bool) {
	if value == nil {
		return nil, false
	}
	if inst, ok := value.(ssa.Instruction); ok && inst.IsLazy() {
		if self, ok := inst.Self().(ssa.Value); ok && self != nil {
			value = self
		}
	}
	if param, ok := ssa.ToParameter(value); ok && param != nil && param.GetDefault() != nil {
		value = param.GetDefault()
	}
	if ssaFn, ok := ssa.ToFunction(value); ok && ssaFn != nil && !ssaFn.IsExtern() {
		return ssaFn, true
	}
	if ft, ok := value.GetType().(*ssa.FunctionType); ok && ft != nil && ft.This != nil && !ft.This.IsExtern() {
		return ft.This, true
	}
	return nil, false
}

func (c *Compiler) materializeCallableClosure(contextInst ssa.Instruction, ssaFn *ssa.Function) (llvm.Value, error) {
	if ssaFn == nil {
		return llvm.Value{}, fmt.Errorf("materializeCallableClosure: missing function")
	}
	llvmFn, _ := c.getOrDeclareLLVMFunction(ssaFn)
	if llvmFn.IsNil() {
		return llvm.Value{}, fmt.Errorf("materializeCallableClosure: failed to declare %s", ssaFn.GetName())
	}
	c.enterMaterializingCallable(ssaFn)
	defer c.leaveMaterializingCallable(ssaFn)

	i64 := c.LLVMCtx.Int64Type()
	i8Ptr := llvm.PointerType(c.LLVMCtx.Int8Type(), 0)
	target := c.Builder.CreatePtrToInt(llvmFn, i64, "yak_callable_fn")
	callerFn := c.currentFunction()
	if contextInst != nil && contextInst.GetFunc() != nil {
		callerFn = contextInst.GetFunc()
	}

	freeValueIDs := c.callableClosureFreeValueIDs(contextInst, ssaFn)
	freeValuesPtr := llvm.ConstPointerNull(i8Ptr)
	if len(freeValueIDs) > 0 {
		mallocFn, mallocType := c.getOrInsertMalloc()
		sizeBytes := llvm.ConstInt(i64, uint64(len(freeValueIDs)*8), false)
		raw := c.Builder.CreateCall(mallocType, mallocFn, []llvm.Value{sizeBytes}, "yak_callable_free_mem")
		i64Ptr := llvm.PointerType(i64, 0)
		freeI64Ptr := c.Builder.CreateIntToPtr(raw, i64Ptr, "yak_callable_free_i64p")
		for index, valueID := range freeValueIDs {
			value := llvm.ConstInt(i64, 0, false)
			if valueID > 0 {
				if capturedFn, ok := c.functionValueForArg(callerFn, valueID); ok && c.isMaterializingCallable(capturedFn) {
					capturedLLVMFn, _ := c.getOrDeclareLLVMFunction(capturedFn)
					if !capturedLLVMFn.IsNil() {
						value = c.Builder.CreatePtrToInt(capturedLLVMFn, i64, "yak_callable_cycle_fn")
					}
				} else {
					resolved, err := c.resolveCallableCaptureValue(contextInst, valueID)
					if err != nil {
						return llvm.Value{}, fmt.Errorf("materializeCallableClosure: free value %d: %w", valueID, err)
					}
					value = c.coerceToInt64(resolved)
				}
			}
			idx := llvm.ConstInt(i64, uint64(index), false)
			slot := c.Builder.CreateGEP(i64, freeI64Ptr, []llvm.Value{idx}, "")
			c.Builder.CreateStore(value, slot)
		}
		freeValuesPtr = c.Builder.CreateBitCast(freeI64Ptr, i8Ptr, "yak_callable_free_i8p")
	}

	makeFn, makeType := c.getOrInsertRuntimeMakeCallable()
	return c.Builder.CreateCall(makeType, makeFn, []llvm.Value{
		target,
		llvm.ConstInt(i64, uint64(len(ssaFn.ParameterMembers)), false),
		llvm.ConstInt(i64, uint64(len(freeValueIDs)), false),
		freeValuesPtr,
	}, "yak_callable_closure"), nil
}

func (c *Compiler) enterMaterializingCallable(fn *ssa.Function) {
	if c == nil || fn == nil || fn.GetId() <= 0 {
		return
	}
	if c.materializingCallableIDs == nil {
		c.materializingCallableIDs = make(map[int64]int)
	}
	c.materializingCallableIDs[fn.GetId()]++
}

func (c *Compiler) leaveMaterializingCallable(fn *ssa.Function) {
	if c == nil || fn == nil || fn.GetId() <= 0 || c.materializingCallableIDs == nil {
		return
	}
	c.materializingCallableIDs[fn.GetId()]--
	if c.materializingCallableIDs[fn.GetId()] <= 0 {
		delete(c.materializingCallableIDs, fn.GetId())
	}
}

func (c *Compiler) isMaterializingCallable(fn *ssa.Function) bool {
	if c == nil || fn == nil || fn.GetId() <= 0 || c.materializingCallableIDs == nil {
		return false
	}
	return c.materializingCallableIDs[fn.GetId()] > 0
}

func (c *Compiler) resolveCallableCaptureValue(contextInst ssa.Instruction, valueID int64) (llvm.Value, error) {
	tagPointerArg := false
	if call, ok := contextInst.(*ssa.Call); ok && call != nil {
		tagPointerArg = c.shouldTagDirectCallArg(call, valueID)
	}
	value, _, err := c.resolveContextCallArg(contextInst, valueID, tagPointerArg)
	if err != nil {
		return llvm.Value{}, err
	}
	return value, nil
}

func (c *Compiler) callableClosureFreeValueIDs(contextInst ssa.Instruction, calleeFn *ssa.Function) []int64 {
	bindings := callframe.OrderedFreeValueBindings(calleeFn)
	if len(bindings) == 0 {
		return nil
	}

	callerFn := c.currentFunction()
	if contextInst != nil && contextInst.GetFunc() != nil {
		callerFn = contextInst.GetFunc()
	}
	call, _ := contextInst.(*ssa.Call)

	out := make([]int64, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, c.resolveCallableFreeValueID(callerFn, call, binding))
	}
	return out
}

func (c *Compiler) resolveCallableFreeValueID(callerFn *ssa.Function, call *ssa.Call, binding callframe.FreeValueBinding) int64 {
	name := binding.Name
	if call != nil && name != "" {
		if actualID, ok := call.Binding[name]; ok && actualID > 0 && valueBelongsToFunction(callerFn, actualID) {
			return actualID
		}
		if actualID := valueIDFromCallScope(call, name, callerFn); actualID > 0 {
			return actualID
		}
	}
	if binding.Variable != nil {
		if value := binding.Variable.GetValue(); value != nil && value.GetId() > 0 {
			if callerFn == nil || value.GetFunc() == callerFn {
				return value.GetId()
			}
		}
	}
	if callerFn != nil && name != "" {
		for variable, valueID := range callerFn.FreeValues {
			if variable != nil && variable.GetName() == name && valueID > 0 {
				return valueID
			}
		}
	}
	if valueBelongsToFunction(callerFn, binding.ValueID) {
		return binding.ValueID
	}
	return 0
}

func valueIDFromCallScope(call *ssa.Call, name string, callerFn *ssa.Function) int64 {
	if call == nil || name == "" || call.GetBlock() == nil || call.GetBlock().ScopeTable == nil {
		return 0
	}
	variable := ssa.ReadVariableFromScopeAndParent(call.GetBlock().ScopeTable, name)
	if variable == nil || variable.GetValue() == nil || variable.GetValue().GetId() <= 0 {
		return 0
	}
	value := variable.GetValue()
	if callerFn != nil && value.GetFunc() != callerFn {
		return 0
	}
	return value.GetId()
}

func valueBelongsToFunction(fn *ssa.Function, valueID int64) bool {
	if fn == nil || valueID <= 0 {
		return false
	}
	value, ok := fn.GetValueById(valueID)
	return ok && value != nil && value.GetFunc() == fn
}
