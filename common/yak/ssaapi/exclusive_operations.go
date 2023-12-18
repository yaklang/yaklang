package ssaapi

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"sync"
)

// GetContextValue can handle context
func (v *Value) GetContextValue(i string) (*Value, bool) {
	return v.runtimeCtx.Get(i)
}

func (v *Value) GetParent() (*Value, bool) {
	return v.GetContextValue("parent")
}

func (v *Value) SetParent(value *Value) *Value {
	v.runtimeCtx.Set("parent", value)
	return v
}

func (v *Value) SetSideEffect(e *Value) *Value {
	if e == nil {
		v.runtimeCtx.Delete("isSizeEffect")
	}
	v.runtimeCtx.Set("isSizeEffect", e)
	return v
}

func (v *Value) GetSideEffect() (*Value, bool) {
	return v.runtimeCtx.Get(`isSizeEffect`)
}

// GetTopDefs desc all of 'Defs' is not used by any other value
func (i *Value) GetTopDefs() Values {
	return i.getTopDefs(nil)
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

func (i *Value) getTopDefs(visited *sync.Map) Values {
	if i == nil {
		return nil
	}

	if visited == nil {
		// phi node will cause dead loop
		// visited can prevent this
		visited = new(sync.Map)
	}
	if ret, ok := i.node.(*ssa.Phi); ok {
		log.Infof("visited phi: %v", ret.String())
		if _, ok := visited.Load(ret); ok {
			// visited phi
			return nil
		}
		visited.Store(i.node, struct{}{})
	}

	switch ret := i.node.(type) {
	case *ssa.Call:
		log.Info("ssa.Call checking...")
		caller := ret.Method
		if caller == nil {
			return Values{i} // return self
		}
		r, _ := i.GetSideEffect()
		return NewValue(caller).SetParent(i).SetSideEffect(r).getTopDefs(visited)
	case *ssa.Function:
		log.Info("ssa.Function checking...")
		var vals Values
		val, ok := i.GetSideEffect()
		if ok {
			// side effect
			varName := val.node.GetName()
			log.Infof("side-effect val: %v", varName)
			effect, ok := ret.SideEffects[varName]
			if !ok {
				return Values{i}
			}
			if ret := NewValue(effect).SetParent(i).getTopDefs(visited); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		} else {
			// handle return
			for _, r := range ret.Return {
				for _, subVal := range r.GetValues() {
					if ret := NewValue(subVal).SetParent(i).getTopDefs(visited); len(ret) > 0 {
						vals = append(vals, ret...)
					}
				}
			}
		}
		if len(vals) == 0 {
			return Values{i} // no return, use undefined
		}
		return vals
	case *ssa.Parameter:
		log.Infof("checking ssa.Parameters...: %v", ret.String())
		parent, ok := i.GetParent()
		if !ok {
			log.Warn("topdefs parameter context error, skip")
			return Values{i}
		}
		if parent.IsFunction() {
			called, ok := parent.GetParent()
			if !ok {
				log.Infof("parent function is not called by any other function, skip")
				return Values{i}
			}
			if !called.IsCall() {
				log.Infof("parent function is not called by any other function, skip (%T)", called)
				return Values{i}
			}
			var vals Values
			calledInstance := called.node.(*ssa.Call)
			for _, i := range calledInstance.Args {
				traced := NewValue(i).SetParent(called)
				if ret := traced.getTopDefs(visited); len(ret) > 0 {
					vals = append(vals, ret...)
				} else {
					vals = append(vals, traced)
				}
			}
			if len(vals) == 0 {
				return Values{NewValue(ssa.NewUndefined("_")).SetParent(i)} // no return, use undefined
			}
			return vals
		} else if parent != i {
			var vals Values
			if ret.IsFreeValue {
				// free value
				// fetch parent
				fun := ret.GetFunc().GetParent() // func.parent
				for _, va := range fun.GetValuesByName(ret.GetName()) {
					_, isSideEffect := va.(*ssa.SideEffect)
					if isSideEffect {
						continue
					}

					if ret := NewValue(va).SetParent(i).getTopDefs(visited); len(ret) > 0 {
						vals = append(vals, ret...)
					}
				}
			}
			if len(vals) <= 0 {
				return Values{i}
			}
			return vals
		}
		return Values{i}
	case *ssa.ConstInst:
		return Values{i}
	case *ssa.Undefined:
		return Values{i}
	case *ssa.Phi:
		log.Infof("handling phi")
		var vars Values
		for _, eg := range ret.Edge {
			if ret := NewValue(eg).SetParent(i).getTopDefs(visited); len(ret) > 0 {
				vars = append(vars, ret...)
			}
		}
		return vars
	case *ssa.SideEffect:
		var vals Values
		for _, subVal := range ret.GetValues() {
			if ret := NewValue(subVal).SetParent(i).SetSideEffect(i).getTopDefs(visited); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		}
		return vals
	default:
		var vals Values
		for _, val := range i.node.GetValues() {
			if ret := NewValue(val).SetParent(i).getTopDefs(visited); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		}
		return vals
	}
	return nil
}
