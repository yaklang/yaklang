package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (v *Value) GetBottomUses() Values {
	return v.getBottomUses(nil)
}

const (
	SSA_BOTTOM_USES_targetActualParam      = "targetActualParam"
	SSA_BOTTOM_USES_targetActualParamIndex = "targetActualParam_Index"
)

func (v *Value) visitUserFallback(actx *AnalyzeContext) Values {
	var vals Values
	for _, user := range v.node.GetUsers() {
		if ret := NewValue(user).SetParent(v).getBottomUses(actx); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	}
	if len(vals) <= 0 {
		return Values{v}
	}
	return vals
}

func (v *Value) getBottomUses(actx *AnalyzeContext) Values {
	if actx == nil {
		actx = NewAnalyzeContext()
	}
	switch ins := v.node.(type) {
	case *ssa.Return:
		// enter function via return
		if actx.CallStack.Len() > 0 {
			val := actx.CallStack.Pop()
			call, ok := val.node.(*ssa.Call)
			if !ok {
				log.Warnf("BUG: (callStack is not clean!) unknown call: %v", v.String())
				return Values{v}
			}
			fun, ok := call.Method.(*ssa.Function)
			if !ok {
				log.Warnf("BUG: (call's fun is not clean!) unknown function: %v", v.String())
				return Values{v}
			}
			_ = fun //TODO: fun can tell u, which return value is the target
			var vals Values
			for _, u := range call.GetUsers() {
				if ret := NewValue(u).SetParent(v).getBottomUses(actx); len(ret) > 0 {
					vals = append(vals, ret...)
				}
			}
			if len(vals) > 0 {
				return vals
			}
			return Values{v}
		}
	case *ssa.Function:
		// enter function via function
		// via call
		// param is set
		if actualParam, ok := v.GetContextValue(SSA_BOTTOM_USES_targetActualParam); ok {
			var shouldHandleTarget int = -1
			if actualParamIndex, ok := v.GetContextValue(SSA_BOTTOM_USES_targetActualParamIndex); ok {
				if targetIndex, ok := actualParamIndex.GetConstValue().(int); ok {
					shouldHandleTarget = targetIndex
					goto NEXT
				}
				log.Warnf("BUG: unknown actual param index: %v", v.String())
				return Values{v}
			}
		NEXT:
			_ = actualParam // TODO: handle actual param, replace value! wait...
			if shouldHandleTarget < 0 {
				log.Warnf("BUG: unknown actual param index: %v", v.String())
			}

			if len(ins.Param) <= shouldHandleTarget {
				log.Warnf("BUG: unknown actual param index: %v", v.String())
				return Values{v}
			}
			val := ins.Param[shouldHandleTarget]
			return NewValue(val).SetParent(v).getBottomUses(actx)
		}
	case *ssa.Call:
		_ = ins
		parent, ok := v.GetParent()
		if !ok {
			log.Warnf("BUG: unknown parent for call: %v", v.String())
			return Values{v}
		}
		if ins.Method != nil {
			if _, undefinedVar := ins.Method.(*ssa.Undefined); undefinedVar {
				return Values{v}
			}
			var targetIndex = -1
			for index, value := range ins.Args {
				if parent.GetId() == value.GetId() {
					targetIndex = index
					break
				}
			}
			if targetIndex == -1 {
				log.Warnf("Wired: the actual param(t%v) is not in call's params list: %v", parent.GetId(), v.String())
				return Values{v}
			}
			nv := NewValue(ins.Method)
			nv.SetContextValue(SSA_BOTTOM_USES_targetActualParam, parent)
			nv.SetContextValue(SSA_BOTTOM_USES_targetActualParamIndex, NewValue(ssa.NewConst(targetIndex)))
			actx.CallStack.Push(v)
			return nv.getBottomUses(actx)
		}
		return v.visitUserFallback(actx)
	}
	return v.visitUserFallback(actx)
}
