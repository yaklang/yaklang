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
	return v.getValue().GetFunc().IsMain()
}

func (v *Value) GetBlock() *Value {
	return v.NewValue(v.getValue().GetBlock())
}

func (v *Value) IsReachable() ssa.BasicBlockReachableKind {
	return v.getValue().GetBlock().Reachable()
}

func (v *Value) GetReachable() *Value {
	node := v.getValue()
	condition, ok := node.GetValueById(node.GetBlock().Condition)
	if !ok {
		condition = nil
	}
	return v.NewValue(condition)
}
