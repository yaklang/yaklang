package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
)

// GetContextValue can handle context
func (v *Value) GetContextValue(i string) (*Value, bool) {
	return v.runtimeCtx.Get(i)
}

func (v *Value) SetContextValue(i string, values *Value) *Value {
	v.runtimeCtx.Set(i, values)
	return v
}

func (v *Value) SetDepth(i int) {
	v.runtimeCtx.Set("depth", NewValue(ssa.NewConst(i)))
}

func (v *Value) GetDepth() int {
	i, ok := v.runtimeCtx.Get("depth")
	if ok {
		return codec.Atoi(i.node.String())
	}
	return 0
}

func (v *Value) AppendDependOn(i *Value) *Value {
	if v.GetId() == i.GetId() {
		return v
	}

	var existed bool
	for _, node := range v.DependOn {
		if node.GetId() == i.GetId() {
			existed = true
			break
		}
	}
	if !existed {
		v.DependOn = append(v.DependOn, i)
	}
	existed = false
	for _, node := range i.EffectOn {
		if node.GetId() == v.GetId() {
			existed = true
			break
		}
	}
	if !existed {
		i.EffectOn = append(i.EffectOn, v)
	}
	return v
}

func (v *Value) AppendEffectOn(i *Value) *Value {
	if v.GetId() == i.GetId() {
		return v
	}

	var existed bool
	for _, node := range v.EffectOn {
		if node.GetId() == i.GetId() {
			existed = true
			break
		}
	}
	if !existed {
		v.EffectOn = append(v.EffectOn, i)
	}
	existed = false
	for _, node := range i.DependOn {
		if node.GetId() == v.GetId() {
			existed = true
			break
		}
	}
	if !existed {
		i.DependOn = append(i.DependOn, v)
	}
	return v
}

// GetTopDefs desc all of 'Defs' is not used by any other value
func (i *Value) GetTopDefs(opt ...OperationOption) Values {
	return i.getTopDefs(nil, opt...)
}

func (v Values) GetTopDefs() Values {
	ret := make(Values, 0, len(v))
	var m = make(map[*Value]struct{})
	v.WalkDefs(func(i *Value) {
		if !i.HasOperands() {
			if _, ok := m[i]; ok {
				return
			}
			m[i] = struct{}{}
			ret = append(ret, i)
		}
	})
	return ret
}

func (i Values) AppendEffectOn(v *Value) Values {
	for _, node := range i {
		node.AppendEffectOn(v)
	}
	return i
}

func (i *Value) visitedDefsDefault(actx *AnalyzeContext) Values {
	var vals Values
	if i.node == nil {
		return vals
	}
	for _, def := range i.node.GetValues() {
		if ret := NewValue(def).AppendEffectOn(i).getTopDefs(actx); len(ret) > 0 {
			vals = append(vals, ret...)
		}
	}

	if len(vals) <= 0 {
		vals = append(vals, i)
	}
	if maskable, ok := i.node.(ssa.Maskable); ok {
		for _, def := range maskable.GetMask() {
			if ret := NewValue(def).AppendEffectOn(i).getTopDefs(actx); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		}
	}
	return vals
}

var (
	ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY = "call_entry"
)

func (i *Value) getTopDefs(actx *AnalyzeContext, opt ...OperationOption) Values {
	if i == nil {
		return nil
	}

	if actx == nil {
		actx = NewAnalyzeContext(opt...)
	}

	actx.depth++
	defer func() {
		actx.depth--
	}()
	i.SetDepth(actx.depth)
	if actx.config.MaxDepth >= 0 && actx.depth > actx.config.MaxDepth {
		return Values{}
	}

	// hook everynode
	if actx.config.HookEveryNode != nil {
		if err := actx.config.HookEveryNode(i); err != nil {
			log.Errorf("hook-every-node error: %v", err)
			return Values{}
		}
	}

	switch ret := i.node.(type) {
	case *ssa.Field:
		return i.visitedDefsDefault(actx)
	case *ssa.Phi:
		if !actx.ThePhiShouldBeVisited(i) {
			// phi is visited...
			return Values{}
		}
		actx.VisitPhi(i)
		return i.visitedDefsDefault(actx)
	case *ssa.Call:
		caller := ret.Method
		if caller == nil {
			return Values{i} // return self
		}

		err := actx.PushCall(i)
		if err != nil {
			log.Errorf("push call error: %v", err)
			return Values{i}
		}
		defer actx.PopCall()

		// TODO: trace the specific return-values
		callerValue := NewValue(caller)
		callerFunc, isFunc := ssa.ToFunction(caller)
		if !isFunc {
			i.AppendDependOn(callerValue)
			var nodes = Values{callerValue}
			for _, val := range ret.Args {
				arg := NewValue(val)
				i.AppendDependOn(arg)
				nodes = append(nodes, arg)
			}
			var results Values
			for _, subNode := range nodes {
				vals := subNode.getTopDefs(actx, opt...).AppendEffectOn(subNode)
				results = append(results, vals...)
			}
			return results
		}
		_ = callerFunc

		callerValue.SetContextValue(ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY, i)
		callerValue.AppendEffectOn(i)
		return callerValue.getTopDefs(actx, opt...).AppendEffectOn(callerValue)
	case *ssa.Function:
		log.Info("ssa.Function checking...")
		var vals Values
		// handle return
		for _, r := range ret.Return {
			for _, subVal := range r.GetValues() {
				if ret := NewValue(subVal).AppendEffectOn(i).getTopDefs(actx); len(ret) > 0 {
					vals = append(vals, ret...)
				}
			}
		}
		if len(vals) == 0 {
			return Values{i} // no return, use undefined
		}
		return vals.AppendEffectOn(i)
	case *ssa.Parameter:
		log.Infof("checking ssa.Parameters...: %v", ret.String())

		called := actx.GetCurrentCall()
		if called == nil {
			log.Error("parent function is not called by any other function, skip")
			return Values{i}
		}
		if !called.IsCall() {
			log.Infof("parent function is not called by any other function, skip (%T)", called)
			return Values{i}
		}
		var vals Values
		calledInstance := called.node.(*ssa.Call)
		for idx, i := range calledInstance.Args {
			if ret.IsFreeValue {
				log.Warn("TODO: Free ssa.Parameters is need to be handled.")
				continue
			}

			if idx != ret.FormalParameterIndex {
				continue
			}

			traced := NewValue(i).AppendEffectOn(called)
			if ret := traced.getTopDefs(actx); len(ret) > 0 {
				vals = append(vals, ret...)
			} else {
				vals = append(vals, traced)
			}
		}
		if len(vals) == 0 {
			return Values{NewValue(ssa.NewUndefined("_")).AppendEffectOn(i)} // no return, use undefined
		}
		return vals.AppendEffectOn(i)
	}
	return i.visitedDefsDefault(actx)
}
