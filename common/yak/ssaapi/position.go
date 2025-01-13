package ssaapi

import "github.com/yaklang/yaklang/common/yak/ssa"

func (v *Value) GetFunction() *Value {
	inst := v.getInstruction()
	if inst == nil {
		return nil
	}
	fun := inst.GetFunc()
	if fun == nil {
		return nil
	}
	return v.NewValue(fun)
}

func (v *Value) InMainFunction() bool {
	return v.innerValue.GetFunc().IsMain()
}

func (v *Value) GetBlock() *Value {
	return v.NewValue(v.innerValue.GetBlock())
}

func (v *Value) IsReachable() ssa.BasicBlockReachableKind {
	return v.innerValue.GetBlock().Reachable()
}

func (v *Value) GetReachable() *Value {
	node := v.innerValue
	return v.NewValue(node.GetValueById(node.GetBlock().Condition))
}
