package ssaapi

import (
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

func (i Values) AppendEffectOn(v *Value) Values {
	for _, node := range i {
		node.AppendEffectOn(v)
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

func (i Values) AppendEffectOn(v *Value) Values {
	for _, node := range i {
		node.AppendEffectOn(v)
	}
	return i
}

func (i Values) AppendDependOn(v *Value) Values {
	for _, node := range i {
		node.AppendDependOn(v)
	}
	return i
}

func (v Values) GetTopDefs(opts ...OperationOption) Values {
	ret := make(Values, 0)
	for _, sub := range v {
		ret = append(ret, sub.GetTopDefs(opts...)...)
	}
	return ret
}

func (v Values) GetBottomUses(opts ...OperationOption) Values {
	ret := make(Values, 0)
	for _, sub := range v {
		ret = append(ret, sub.GetBottomUses(opts...)...)
	}
	return ret
}

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
