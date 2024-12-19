package ssaapi

import (
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/utils"
	"github.com/yaklang/yaklang/common/yak/ssa"
	"github.com/yaklang/yaklang/common/yak/yaklib/codec"
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

func (i Values) AppendEffectOn(vs ...*Value) Values {
	for _, v := range vs {
		for _, node := range i {
			node.AppendEffectOn(v)
		}
	}
	return i
}

func (i Values) AppendDependOn(vs ...*Value) Values {
	for _, v := range vs {
		for _, node := range i {
			node.AppendDependOn(v)
		}
	}
	return i
}

type ContextID string

var (
	ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY             ContextID = "call_entry"
	ANALYZE_RUNTIME_CTX_TOPDEF_CALL_ENTRY_TRACE_INDEX ContextID = "call_entry_trace_idx"
)

// GetContextValue can handle context
func (v *Value) GetContextValue(i ContextID) (*Value, bool) {
	return v.runtimeCtx.Get(i)
}

func (v *Value) SetContextValue(i ContextID, values *Value) *Value {
	v.runtimeCtx.Set(i, values)
	return v
}

func (v *Value) SetDepth(i int) {
	v.runtimeCtx.Set("depth", v.NewValue(ssa.NewConst(i)))
}

func (v *Value) GetDepth() int {
	i, ok := v.runtimeCtx.Get("depth")
	if ok {
		return codec.Atoi(i.node.String())
	}
	return 0
}

func (i *Value) AppendDependOn(vs ...*Value) *Value {
	for _, v := range vs {
		if i.GetId() == v.GetId() {
			return i
		}
		i.DependOn = utils.AppendSliceItemWhenNotExists(i.DependOn, v)
		v.EffectOn = utils.AppendSliceItemWhenNotExists(v.EffectOn, i)
	}
	return i
}

func (i *Value) AppendEffectOn(vs ...*Value) *Value {
	for _, v := range vs {
		if i.GetId() == v.GetId() {
			return i
		}
		i.EffectOn = utils.AppendSliceItemWhenNotExists(i.EffectOn, v)
		v.DependOn = utils.AppendSliceItemWhenNotExists(v.DependOn, i)
	}
	return i
}

func MergeValues(vs ...Values) Values {
	tmp := make(map[int64]*Value)
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
		if exist, has := tmp[v.GetId()]; has {
			// merge v to existValue
			for _, effect := range v.EffectOn {
				effect.DependOn = utils.RemoveSliceItem(effect.DependOn, v)
				exist.AppendEffectOn(effect)
			}

			for _, depend := range v.DependOn {
				depend.EffectOn = utils.RemoveSliceItem(depend.EffectOn, v)
				exist.AppendDependOn(depend)
			}

			for _, pred := range v.Predecessors {
				exist.Predecessors = utils.AppendSliceItemWhenNotExists(exist.Predecessors, pred)
			}
		} else {
			// set v is exist
			tmp[v.GetId()] = v
		}
	}
	for _, vs := range vs {
		for _, v := range vs {
			checkAndMerge(v)
		}
	}
	return append(lo.Values(tmp), templateValue...)
}
