package ssaapi

func (v *Value) GetFunction() *Value {
	return NewValue(v.node.GetFunc())
}

func (v *Value) InMainFunction() bool {
	return v.node.GetFunc().IsMain()
}

func (v *Value) GetBlock() *Value {
	return NewValue(v.node.GetBlock())
}

/*
if condition is true  :  1 reach
if condition is false : -1 unreachable
if condition need calc: 0  unknown
*/
func (v *Value) IsReachable() int {
	return v.node.GetBlock().Reachable()
}

func (v *Value) GetReachable() *Value {
	return NewValue(v.node.GetBlock().Condition)
}
