package ssaapi

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Values []*Value

func (value Values) Ref(name string) Values {
	// return nil
	var ret Values
	for _, v := range value {
		v.GetUsers().ForEach(func(v *Value) {
			// get value.Name or value["name"]
			if v.IsField() && v.GetOperand(1).String() == name {
				ret = append(ret, v)
			}
		})
	}
	return getValuesWithUpdate(ret)
}

func (v Values) StringEx(flag int) string {
	ret := ""
	ret += fmt.Sprintf("Values: %d\n", len(v))
	for i, v := range v {
		switch flag {
		case 0:
			ret += fmt.Sprintf("\t%d: %5s: %s\n", i, v.node.GetOpcode(), v)
		case 1:
			ret += fmt.Sprintf("\t%d: %s\n", i, v.StringWithSource())
		}
	}
	return ret
}

func (v Values) Show()           { fmt.Println(v.StringEx(0)) }
func (v Values) ShowWithSource() { fmt.Println(v.StringEx(1)) }

func (v Values) Get(i int) *Value { return v[i] }

func (v Values) ForEach(f func(*Value)) {
	for _, v := range v {
		f(v)
	}
}

type Value struct {
	node ssa.InstructionNode
}

func NewValue(n ssa.InstructionNode) *Value {
	return &Value{
		node: n,
	}
}
func (v *Value) String() string { return v.node.LineDisasm() }
func (i *Value) StringWithSource() string {
	return fmt.Sprintf("[%-6s] %s\t%s", i.node.GetOpcode(), i.node.LineDisasm(), i.node.GetPosition())
}
func (i *Value) Show()           { fmt.Println(i) }
func (i *Value) ShowWithSource() { fmt.Println(i.StringWithSource()) }

func (v *Value) IsSame(other *Value) bool { return *v == *other }

func (i *Value) HasOperands() bool {
	return i.node.HasValues()
}

func (i *Value) GetOperands() Values {
	return lo.Map(ssa.GetValues(i.node), func(v ssa.Value, _ int) *Value { return NewValue(v) })
}

func (i *Value) GetOperand(index int) *Value {
	return NewValue(ssa.GetValues(i.node)[index])
}

func (i *Value) GetRawUsers() ssa.Users {
	return i.node.GetUsers()
}

func (i *Value) HasUsers() bool {
	return i.node.HasUsers()
}

func (i *Value) GetUsers() Values {
	return lo.Map(i.GetRawUsers(), func(v ssa.User, _ int) *Value { return NewValue(v) })
}

func (i *Value) GetUser(index int) *Value {
	return NewValue(i.node.GetUsers()[index])
}

func (value *Value) ShowUseDefChain() {
	defaultUseDefChain(value).Show()
}

// for function

func (v *Value) GetReturn() Values {
	ret := make(Values, 0)
	if f, ok := ssa.ToFunction(v.node); ok {
		for _, r := range f.Return {
			ret = append(ret, NewValue(r))
		}
	}
	return ret
}

func (v *Value) GetParameter() Values {
	ret := make(Values, 0)
	if f, ok := ssa.ToFunction(v.node); ok {
		for _, v := range f.Param {
			ret = append(ret, NewValue(v))
		}
	}
	return ret
}

func (v *Value) IsFunction() bool     { return v.node.GetOpcode() == ssa.OpFunction }
func (v *Value) IsBasicBlock() bool   { return v.node.GetOpcode() == ssa.OpBasicBlock }
func (v *Value) IsPhi() bool          { return v.node.GetOpcode() == ssa.OpPhi }
func (v *Value) IsConstInst() bool    { return v.node.GetOpcode() == ssa.OpConstInst }
func (v *Value) IsUndefined() bool    { return v.node.GetOpcode() == ssa.OpUndefined }
func (v *Value) IsBinOp() bool        { return v.node.GetOpcode() == ssa.OpBinOp }
func (v *Value) IsUnOp() bool         { return v.node.GetOpcode() == ssa.OpUnOp }
func (v *Value) IsCall() bool         { return v.node.GetOpcode() == ssa.OpCall }
func (v *Value) IsReturn() bool       { return v.node.GetOpcode() == ssa.OpReturn }
func (v *Value) IsMake() bool         { return v.node.GetOpcode() == ssa.OpMake }
func (v *Value) IsField() bool        { return v.node.GetOpcode() == ssa.OpField }
func (v *Value) IsUpdate() bool       { return v.node.GetOpcode() == ssa.OpUpdate }
func (v *Value) IsNext() bool         { return v.node.GetOpcode() == ssa.OpNext }
func (v *Value) IsAssert() bool       { return v.node.GetOpcode() == ssa.OpAssert }
func (v *Value) IsTypeCast() bool     { return v.node.GetOpcode() == ssa.OpTypeCast }
func (v *Value) IsTypeValue() bool    { return v.node.GetOpcode() == ssa.OpTypeValue }
func (v *Value) IsErrorHandler() bool { return v.node.GetOpcode() == ssa.OpErrorHandler }
func (v *Value) IsPanic() bool        { return v.node.GetOpcode() == ssa.OpPanic }
func (v *Value) IsRecover() bool      { return v.node.GetOpcode() == ssa.OpRecover }
func (v *Value) IsJump() bool         { return v.node.GetOpcode() == ssa.OpJump }
func (v *Value) IsIf() bool           { return v.node.GetOpcode() == ssa.OpIf }
func (v *Value) IsLoop() bool         { return v.node.GetOpcode() == ssa.OpLoop }
func (v *Value) IsSwitch() bool       { return v.node.GetOpcode() == ssa.OpSwitch }
