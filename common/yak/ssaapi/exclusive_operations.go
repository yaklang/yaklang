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
	if visited == nil {
		visited = new(sync.Map)
	}

	switch ret := i.node.(type) {
	case *ssa.Call:
		log.Info("ssa.Call checking...")
		caller := ret.Method
		if caller == nil {
			return Values{i} // return self
		}
		return NewValue(caller).SetParent(i).getTopDefs(visited)
	case *ssa.Function:
		log.Info("ssa.Function checking...")
		var vals Values
		for _, v := range ret.ReturnValue() {
			if ret := NewValue(v).SetParent(i).GetTopDefs(); len(ret) > 0 {
				vals = append(vals, ret...)
			}
		}
		if len(vals) == 0 {
			return Values{NewValue(ssa.NewUndefined("_")).SetParent(i)} // no return, use undefined
		}
		return vals
	case *ssa.Parameter:
		log.Infof("checking ssa.Parameters...: %v", ret.String())
		parentMustFunc, ok := i.GetParent()
		if !ok {
			log.Warn("topdefs parameter context error, skip")
			return Values{i}
		}
		if !parentMustFunc.IsFunction() {
			log.Infof("parent is not function, skip")
			return Values{i}
		}
		called, ok := parentMustFunc.GetParent()
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
			if ret := traced.GetTopDefs(); len(ret) > 0 {
				vals = append(vals, ret...)
			} else {
				vals = append(vals, traced)
			}
		}
		if len(vals) == 0 {
			return Values{NewValue(ssa.NewUndefined("_")).SetParent(i)} // no return, use undefined
		}
		return vals
	}
	return nil
}
