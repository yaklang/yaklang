package compiler

import (
	"fmt"

	"github.com/yaklang/go-llvm"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (c *Compiler) compileSideEffect(inst *ssa.SideEffect) error {
	if inst == nil {
		return nil
	}
	if inst.IsMember() && inst.GetObject() != nil && inst.GetKey() != nil {
		objVal, err := c.getValue(inst, inst.GetObject().GetId())
		if err != nil {
			return err
		}
		keyStr := c.resolveMemberKeyString(inst.GetKey())
		if keyStr == "" {
			c.cacheValue(inst.GetId(), llvm.ConstInt(c.LLVMCtx.Int64Type(), 0, false))
			return nil
		}
		if actualID, err := c.resolveSideEffectActualID(inst); err == nil && actualID > 0 && actualID != inst.GetId() && valueBelongsToFunction(inst.GetFunc(), actualID) {
			actualVal, err := c.getValue(inst, actualID)
			if err == nil && !actualVal.IsNil() {
				c.cacheValue(inst.GetId(), actualVal)
				c.emitRuntimeSetField(objVal, keyStr, actualVal, c.assignedSSAValue(inst, actualID), inst.GetId())
				return c.maybeEmitMemberSet(inst, inst, inst.GetId())
			}
		}
		c.cacheValue(inst.GetId(), c.emitRuntimeGetField(objVal, keyStr, inst.GetId()))
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
		c.cacheValue(inst.GetId(), c.emitRuntimeGetField(objVal, keyStr, inst.GetId()))
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}
	if val, ok := c.getCachedValue(inst, inst.GetId()); ok && !val.IsNil() {
		return c.maybeEmitMemberSet(inst, inst, inst.GetId())
	}
	actualID, err := c.resolveSideEffectActualID(inst)
	if err != nil {
		return err
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

func coerceLLVMValueSliceToI64(c *Compiler, vals []llvm.Value) []llvm.Value {
	if c == nil {
		return vals
	}
	out := make([]llvm.Value, 0, len(vals))
	for _, v := range vals {
		out = append(out, c.coerceToInt64(v))
	}
	return out
}
