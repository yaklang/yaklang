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
	fn := inst.GetFunc()
	if fn == nil {
		return fmt.Errorf("compileSideEffect: missing function for side-effect %d", inst.GetId())
	}

	callInstAny, ok := fn.GetInstructionById(inst.CallSite)
	if !ok || callInstAny == nil {
		return fmt.Errorf("compileSideEffect: callsite %d not found", inst.CallSite)
	}
	callInst, ok := callInstAny.(*ssa.Call)
	if !ok || callInst == nil {
		return fmt.Errorf("compileSideEffect: callsite %d is %T, want *ssa.Call", inst.CallSite, callInstAny)
	}

	actualID := inst.Value
	if valueAny, ok := fn.GetValueById(inst.Value); ok && valueAny != nil {
		switch tmpl := valueAny.(type) {
		case *ssa.Parameter:
			idx := tmpl.FormalParameterIndex
			if idx >= 0 && idx < len(callInst.Args) {
				actualID = callInst.Args[idx]
			} else {
				return fmt.Errorf("compileSideEffect: parameter index %d out of bounds for call %d (args=%d)", idx, callInst.GetId(), len(callInst.Args))
			}
		case *ssa.ParameterMember:
			if actual, ok := tmpl.GetActualCallParam(callInst); ok && actual != nil {
				actualID = actual.GetId()
			} else {
				return fmt.Errorf("compileSideEffect: failed to resolve actual call param for %s at call %d", tmpl.GetName(), callInst.GetId())
			}
		}
	}

	actualVal, err := c.getValue(inst, actualID)
	if err != nil {
		return err
	}
	actualVal = c.coerceToInt64(actualVal)
	if inst.GetId() > 0 {
		c.Values[inst.GetId()] = actualVal
	}

	// SideEffect values are used for SSA-level member tracking and are passed
	// to callees via Call.ArgMember. Do not emit runtime field mutation here.
	return nil
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

