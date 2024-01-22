package ssaapi

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
