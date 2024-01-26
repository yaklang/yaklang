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
		if ret := NewValue(user).AppendDependOn(v).getBottomUses(actx); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	}
	if len(vals) <= 0 {
		return Values{v}
	}
	return vals
}

func (v *Value) getBottomUses(actx *AnalyzeContext, opt ...OperationOption) Values {
	if actx == nil {
		actx = NewAnalyzeContext(opt...)
	}

	actx.depth++
	defer func() {
		actx.depth--
	}()
	v.SetDepth(actx.depth)
	if actx.config.MaxDepth > 0 && actx.depth > actx.config.MaxDepth {
		return Values{}
	}
	if actx.config.MinDepth < 0 && actx.depth < actx.config.MinDepth {
		return Values{}
	}

	if actx.config.HookEveryNode != nil {
		err := actx.config.HookEveryNode(v)
		if err != nil {
			log.Errorf("hook every node failed: %v", err)
		}
	}

	switch ins := v.node.(type) {
	case *ssa.Phi:
		// enter function via phi
		if !actx.ThePhiShouldBeVisited(v) {
			// the phi is existed, visited in the same stack.
			return Values{}
		}
		actx.VisitPhi(v)
		return v.visitUserFallback(actx)
	case *ssa.Return:
		// enter function via return
		fallback := func() Values {
			var results Values
			for _, result := range ins.Results {
				results = append(results, NewValue(result).AppendDependOn(v))
			}
			return results
		}
		if actx._callStack.Len() > 0 {
			val := actx.GetCurrentCall()
			if val == nil {
				return fallback()
			}
			call := val.node.(*ssa.Call)
			fun, ok := call.Method.(*ssa.Function)
			if !ok {
				log.Warnf("BUG: (call's fun is not clean!) unknown function: %v", v.String())
				return fallback()
			}
			_ = fun //TODO: fun can tell u, which return value is the target
			var vals Values
			for _, u := range call.GetUsers() {
				if ret := NewValue(u).AppendDependOn(v).getBottomUses(actx); len(ret) > 0 {
					vals = append(vals, ret...)
				}
			}
			if len(vals) > 0 {
				return vals
			}
			return NewValue(call).AppendDependOn(v).getBottomUses(actx)
		}
		return fallback()
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
			return NewValue(val).AppendDependOn(v).getBottomUses(actx)
		}
	case *ssa.Call:
		if !actx.TheCallShouldBeVisited(v) {
			// call existed
			return v.visitUserFallback(actx)
		}

		log.Errorf("BUG: (callStack is not clean!) unknown call: %v", v.String())
		return v.visitUserFallback(actx)
		//
		//parent, ok := v.GetParent()
		//if !ok {
		//	log.Warnf("BUG: unknown parent for call: %v", v.String())
		//	return Values{v}
		//}
		//if ins.Method != nil {
		//	if _, undefinedVar := ins.Method.(*ssa.Undefined); undefinedVar {
		//		return Values{v}
		//	}
		//	var targetIndex = -1
		//	for index, value := range ins.Args {
		//		if parent.GetId() == value.GetId() {
		//			targetIndex = index
		//			break
		//		}
		//	}
		//	if targetIndex == -1 {
		//		log.Warnf("Wired: the actual param(t%v) is not in call's params list: %v", parent.GetId(), v.String())
		//		return Values{v}
		//	}
		//	nv := NewValue(ins.Method)
		//	nv.SetContextValue(SSA_BOTTOM_USES_targetActualParam, parent)
		//	nv.SetContextValue(SSA_BOTTOM_USES_targetActualParamIndex, NewValue(ssa.NewConst(targetIndex)))
		//	err := actx.PushCall(v)
		//	if err != nil {
		//		log.Warnf("BUG: (callStack is not clean!) push callStack failed: %T", v.node)
		//		return v.visitUserFallback(actx)
		//	}
		//	defer actx.PopCall()
		//	return nv.getBottomUses(actx)
		//}
		//return v.visitUserFallback(actx)
	}
	return v.visitUserFallback(actx)
}
