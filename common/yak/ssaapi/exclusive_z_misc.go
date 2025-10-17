package ssaapi

import (
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/utils/omap"
)

func (v *Value) LoadFullUseDefChain() *Value {
	v.GetTopDefs()
	v.GetBottomUses()
	return v
}

func (v Values) FullUseDefChain(h func(*Value)) {
	for _, val := range v {
		h(val.LoadFullUseDefChain())
	}
}

func (i Values) AppendEffectOn(vs *Value) Values {
	for _, node := range i {
		node.AppendEffectOn(vs)
	}
	return i
}

func (i Values) AppendDependOn(v *Value) Values {
	for _, node := range i {
		node.AppendDependOn(v)
	}
	return i
}

type ContextID string

var (
	ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY             ContextID = "call_entry"
	ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX ContextID = "call_entry_trace_idx"
)

func NewRuntimeContext() *omap.OrderedMap[ContextID, *Value] {
	return omap.NewEmptyOrderedMap[ContextID, *Value]()
}

// GetContextValue can handle context
func (v *Value) GetContextValue(i ContextID) (*Value, bool) {
	if v.runtimeCtx == nil {
		v.runtimeCtx = NewRuntimeContext()
	}
	return v.runtimeCtx.Get(i)
}

func (v *Value) SetContextValue(i ContextID, values *Value) *Value {
	if v.runtimeCtx == nil {
		v.runtimeCtx = NewRuntimeContext()
	}
	v.runtimeCtx.Set(i, values)
	return v
}

func (i *Value) AppendDependOn(v *Value) (ret *Value) {
	ret = i
	if i == nil {
		return i
	}
	if v == nil {
		return
	}
	if i.GetUUID() == v.GetUUID() {
		return
	}
	if i.hasDependOn(v) {
		return
	} else {
		i.setDependOn(v)
		v.setEffectOn(i)
	}
	return i
}

func (i *Value) AppendEffectOn(v *Value) (ret *Value) {
	ret = i
	if i == nil {
		return i
	}
	if v == nil {
		return
	}
	if i.GetUUID() == v.GetUUID() {
		return
	}
	if i.hasEffectOn(v) {

	} else {
		i.setEffectOn(v)
		v.setDependOn(i)
	}
	return i
}

func (i *Value) RemoveDependOn(vs ...*Value) {
	if i == nil {
		return
	}
	for _, v := range vs {
		if v == nil {
			continue
		}
		if i.GetUUID() == v.GetUUID() {
			continue
		}
		if i.hasDependOn(v) {
			i.deleteDependOn(v)
		} else {
			continue
		}
	}
	return
}

func (i *Value) RemoveEffectOn(vs ...*Value) {
	if i == nil {
		return
	}
	for _, v := range vs {
		if v == nil {
			continue
		}
		if i.GetUUID() == v.GetUUID() {
			continue
		}
		if i.hasEffectOn(v) {
			i.deleteEffectOn(v)
		} else {
			continue
		}
	}
	return
}

func MergeValues(allVs ...Values) Values {
	tmp := omap.NewEmptyOrderedMap[int64, *Value]()
	templateValue := make(Values, 0)
	checkAndMerge := func(v *Value) {
		if utils.IsNil(v) {
			return
		}
		// template value will not merge, this value create in query Runtime
		if v.GetId() == -1 {
			templateValue = append(templateValue, v)
			return
		}
		if existValue, exist := tmp.Get(v.GetId()); exist {
			// merge v to exist value
			if v.EffectOn != nil {
				v.EffectOn.ForEach(func(key string, effect *Value) bool {
					effect.RemoveDependOn(v)
					existValue.AppendEffectOn(effect)
					return true
				})
			}

			if v.DependOn != nil {
				v.DependOn.ForEach(func(key string, depend *Value) bool {
					depend.RemoveEffectOn(v)
					existValue.AppendDependOn(depend)
					return true
				})
			}
			for _, pred := range v.Predecessors {
				existValue.Predecessors = utils.AppendSliceItemWhenNotExists(existValue.Predecessors, pred)
			}
		} else {
			// set v is exist
			tmp.Set(v.GetId(), v)
		}
	}

	for _, vs := range allVs {
		for _, v := range vs {
			checkAndMerge(v)
		}
	}
	values := append(tmp.Values(), templateValue...)
	return values
}
