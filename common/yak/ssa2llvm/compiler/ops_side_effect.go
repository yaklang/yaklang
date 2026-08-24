package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/callframe"
	"github.com/yaklang/yaklang/common/yak/ssa2llvm/runtime/abi"
)

func (c *Compiler) compileSideEffectInstruction(inst *ssa.SideEffect) error {
	if c.isAsyncSideEffect(inst) {
		return nil
	}
	return c.compileSideEffectValue(inst)
}

func (c *Compiler) compileSideEffectValue(inst *ssa.SideEffect) error {
	if inst == nil {
		return nil
	}
	if inst.IsMember() && inst.GetObject() != nil && inst.GetKey() != nil {
		// The side-effect writes at its definition site, which is the FIRST
		// owner pair. GetObject() returns the latest owner: when the written
		// value is later reused (e.g. params.language also feeds
		// result["language"]), that latest owner is a forward reference and
		// pulling it here would emit later calls before the write.
		obj, key := c.firstOwnerObjectKey(inst)
		if obj == nil || key == nil {
			obj, key = inst.GetObject(), inst.GetKey()
		}
		objVal, err := c.getValue(inst, obj.GetId())
		if err != nil {
			return err
		}
		keyStr := c.resolveMemberKeyString(key)
		if keyStr == "" {
			c.cacheValue(inst.GetId(), llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
			return nil
		}
		if actualID, err := c.resolveSideEffectActualID(inst); err == nil && actualID > 0 && actualID != inst.GetId() && valueBelongsToFunction(inst.GetFunc(), actualID) {
			actualVal, err := c.getValue(inst, actualID)
			if err == nil && !actualVal.IsNil() {
				c.cacheValue(inst.GetId(), actualVal)
				c.emitRuntimeSetFieldByKey(inst, objVal, key, keyStr, actualVal, c.assignedSSAValue(inst, actualID), inst.GetId())
				return c.maybeEmitMemberSet(inst, inst, inst.GetId())
			}
		}
		c.cacheValue(inst.GetId(), c.emitRuntimeGetFieldByKey(objVal, key, inst, inst.GetId()))
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}
	if actual := c.resolveSideEffectActualValue(inst); actual != nil && actual.IsMember() && actual.GetObject() != nil && actual.GetKey() != nil {
		objVal, err := c.getValue(inst, actual.GetObject().GetId())
		if err != nil {
			return err
		}
		keyStr := c.resolveMemberKeyString(actual.GetKey())
		if keyStr == "" {
			c.cacheValue(inst.GetId(), llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
			return nil
		}
		c.cacheValue(inst.GetId(), c.emitRuntimeGetFieldByKey(objVal, actual.GetKey(), inst, inst.GetId()))
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}
	if val, ok := c.getCachedValue(inst, inst.GetId()); ok && !val.IsNil() {
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}
	actualID, err := c.resolveSideEffectActualID(inst)
	if err != nil {
		return err
	}

	// A side effect after a closure call (e.g. `f(); count` or
	// `retry(100, f); count` where f mutates the captured count) refers to a
	// value owned by the closure function. Resolve it through the closure's
	// free-value slots instead of the cross-function fallback (which returns 0).
	closureVal, closureIndex, closureByRef, closureOK := c.resolveClosureSideEffect(inst, actualID)
	if closureOK {
		if closureByRef {
			// Redirect this side-effect's storage to the closure's heap
			// free-value slot: reads at ANY later program point (including
			// after the closure is invoked indirectly through a wrapper such
			// as filesys.onFileStat -> Recursive) observe the final value.
			slotFn, slotType := c.getOrInsertRuntimeGetClosureFreeSlot()
			idx := llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(closureIndex), false)
			slotRaw := c.Builder.CreateCall(slotType, slotFn, []llvm.Value{
				c.coerceToInt64(closureVal), idx,
			}, fmt.Sprintf("yak_closure_fv_slot_%d", inst.GetId()))
			i64Ptr := llvm.PointerType(c.LLVMCtx.Int64Type(), 0)
			heapPtr := c.Builder.CreateIntToPtr(c.coerceToInt64(slotRaw), i64Ptr, fmt.Sprintf("yak_closure_fv_slotp_%d", inst.GetId()))
			if c.function.redirectedSlots == nil {
				c.function.redirectedSlots = make(map[int64]llvm.Value)
			}
			c.function.redirectedSlots[inst.GetId()] = heapPtr
			if c.function.storedValues == nil {
				c.function.storedValues = make(map[int64]struct{})
			}
			c.function.storedValues[inst.GetId()] = struct{}{}
			return c.maybeEmitMemberSet(inst, inst, inst.GetId())
		}
		readFn, readType := c.getOrInsertRuntimeReadClosureFreeValue()
		idx := llvm.ConstInt(c.LLVMCtx.Int64Type(), uint64(closureIndex), false)
		byRefWord := llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false)
		val := c.Builder.CreateCall(readType, readFn, []llvm.Value{
			c.coerceToInt64(closureVal), idx, byRefWord,
		}, fmt.Sprintf("yak_closure_fv_read_%d", inst.GetId()))
		val = c.coerceToInt64(val)
		if inst.GetId() > 0 {
			c.cacheValue(inst.GetId(), val)
		}
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}

	actualVal, err := c.getValue(inst, actualID)
	if err != nil {
		return err
	}
	actualVal = c.coerceToInt64(actualVal)
	if inst.GetId() > 0 {
		c.cacheValue(inst.GetId(), actualVal)
	}

	return c.maybeEmitMemberSet(inst, inst, inst.GetId())
}

// resolveClosureSideEffect resolves a side-effect whose modified value belongs
// to a closure function called at the side-effect's call site (either directly
// as the call target or passed as a callback argument). It returns the
// materialized closure object, the free-value binding index for the captured
// variable, whether the capture is by-reference, and ok.
func (c *Compiler) resolveClosureSideEffect(inst *ssa.SideEffect, actualID int64) (llvm.Value, uint64, bool, bool) {
	if inst == nil || inst.CallSite <= 0 || actualID <= 0 || c.function == nil {
		return llvm.Value{}, 0, false, false
	}
	fn := inst.GetFunc()
	if fn == nil {
		return llvm.Value{}, 0, false, false
	}
	valObj, ok := fn.GetValueById(actualID)
	if !ok || valObj == nil {
		return llvm.Value{}, 0, false, false
	}
	closureFn := valObj.GetFunc()
	if closureFn == nil || closureFn == fn {
		return llvm.Value{}, 0, false, false
	}
	// The modified value's variable name comes from the side-effect
	// instruction's own variable bindings (e.g. `files` for
	// `files = append(files, x)`), not from the modified value's id: the
	// value belongs to the closure function and its id is the closure-local
	// append result, which never equals the caller's free-value binding id.
	modifyName := ""
	if vars := inst.GetAllVariables(); len(vars) > 0 {
		for name := range vars {
			modifyName = name
			break
		}
	}
	if modifyName == "" {
		if last := inst.GetLastVariable(); last != nil {
			modifyName = last.GetName()
		}
	}
	if modifyName == "" {
		return llvm.Value{}, 0, false, false
	}
	index := -1
	byRef := false
	for i, binding := range callframe.OrderedFreeValueBindings(closureFn) {
		if binding.Variable.GetName() != modifyName {
			continue
		}
		index = i
		byRef = c.freeValueCaptureMode(closureFn, binding) != freeValueCaptureByValue
		break
	}
	if index < 0 {
		return llvm.Value{}, 0, false, false
	}
	// Direct closure target.
	if c.function.materializedClosures != nil {
		if closureVal, ok := c.function.materializedClosures[inst.CallSite]; ok && !closureVal.IsNil() {
			return closureVal, uint64(index), byRef, true
		}
	}
	// Closure passed as a callback argument (extern/yaklib callee).
	if c.function.materializedClosureArgs != nil {
		for _, closureVal := range c.function.materializedClosureArgs[inst.CallSite] {
			if !closureVal.IsNil() {
				return closureVal, uint64(index), byRef, true
			}
		}
	}
	return llvm.Value{}, 0, false, false
}

func (c *Compiler) getOrInsertRuntimeGetClosureFreeSlot() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeGetClosureFreeSlotSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) getOrInsertRuntimeReadClosureFreeValue() (llvm.Value, llvm.Type) {
	name := c.runtimeSymName(abi.RuntimeReadClosureFreeValueSymbol)
	fn := c.Mod.NamedFunction(name)
	i64 := c.LLVMCtx.Int64Type()
	fnType := llvm.FunctionType(i64, []llvm.Type{i64, i64, i64}, false)
	if fn.IsNil() {
		fn = llvm.AddFunction(c.Mod, name, fnType)
	}
	return fn, fnType
}

func (c *Compiler) isAsyncSideEffect(inst *ssa.SideEffect) bool {
	callInst, ok := c.sideEffectCallSite(inst)
	return ok && callInst.Async
}

func (c *Compiler) sideEffectCallSite(inst *ssa.SideEffect) (*ssa.Call, bool) {
	if inst == nil || inst.CallSite <= 0 || inst.GetFunc() == nil {
		return nil, false
	}
	callInstAny, ok := inst.GetFunc().GetInstructionById(inst.CallSite)
	if !ok || callInstAny == nil {
		return nil, false
	}
	callInst, ok := callInstAny.(*ssa.Call)
	return callInst, ok && callInst != nil
}

func (c *Compiler) resolveSideEffectActualID(inst *ssa.SideEffect) (int64, error) {
	if inst == nil {
		return 0, fmt.Errorf("resolveSideEffectActualID: nil side-effect")
	}
	fn := inst.GetFunc()
	if fn == nil {
		return 0, fmt.Errorf("compileSideEffect: missing function for side-effect %d", inst.GetId())
	}

	callInstAny, ok := fn.GetInstructionById(inst.CallSite)
	if !ok || callInstAny == nil {
		return 0, fmt.Errorf("compileSideEffect: callsite %d not found", inst.CallSite)
	}
	callInst, ok := callInstAny.(*ssa.Call)
	if !ok || callInst == nil {
		return 0, fmt.Errorf("compileSideEffect: callsite %d is %T, want *ssa.Call", inst.CallSite, callInstAny)
	}

	actualID := inst.Value
	if valueAny, ok := fn.GetValueById(inst.Value); ok && valueAny != nil {
		switch tmpl := valueAny.(type) {
		case *ssa.Parameter:
			idx := tmpl.FormalParameterIndex
			if idx >= 0 && idx < len(callInst.Args) {
				actualID = callInst.Args[idx]
			} else {
				return 0, fmt.Errorf("compileSideEffect: parameter index %d out of bounds for call %d (args=%d)", idx, callInst.GetId(), len(callInst.Args))
			}
		case *ssa.ParameterMember:
			if actual, ok := tmpl.GetActualCallParam(callInst); ok && actual != nil {
				actualID = actual.GetId()
			} else {
				return 0, fmt.Errorf("compileSideEffect: failed to resolve actual call param for %s at call %d", tmpl.GetName(), callInst.GetId())
			}
		}
	}
	return actualID, nil
}

func (c *Compiler) resolveSideEffectActualValue(inst *ssa.SideEffect) ssa.Value {
	actualID, err := c.resolveSideEffectActualID(inst)
	if err != nil || actualID <= 0 {
		return nil
	}
	fn := inst.GetFunc()
	if fn == nil {
		return nil
	}
	value, ok := fn.GetValueById(actualID)
	if !ok || value == nil {
		return nil
	}
	return value
}
