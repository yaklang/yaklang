package ssaapi

import "github.com/yaklang/yaklang/common/yak/ssa"

func (v *Value) GetFunction() *Value {
	return v.NewValue(v.node.GetFunc())
}

func (v *Value) InMainFunction() bool {
	return v.node.GetFunc().IsMain()
}

func (v *Value) GetBlock() *Value {
	return v.NewValue(v.node.GetBlock())
}

func (v *Value) IsReachable() ssa.BasicBlockReachableKind {
	return v.node.GetBlock().Reachable()
}

func (v *Value) GetReachable() *Value {
	return v.NewValue(v.node.GetBlock().Condition)
}
