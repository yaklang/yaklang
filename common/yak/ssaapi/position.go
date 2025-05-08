package ssaapi

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

/*
if condition is true  :  1 reach
if condition is false : -1 unreachable
if condition need calc: 0  unknown
*/
func (v *Value) IsReachable() int {
	return v.innerValue.GetBlock().Reachable()
}

func (v *Value) GetReachable() *Value {
	return v.NewValue(v.innerValue.GetBlock().Condition)
}
