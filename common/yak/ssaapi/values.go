package ssaapi

import (
	"fmt"

	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Value struct {
	ssa.InstructionNode
}

func NewValue(n ssa.InstructionNode) *Value {
	return &Value{
		InstructionNode: n,
	}
}
func (v *Value) String() string { return v.LineDisasm() }
func (i *Value) Show()          { fmt.Println(i) }

func (i *Value) GetOperands() Values {
	return lo.Map(i.GetValues(), func(v ssa.Value, _ int) *Value { return NewValue(v) })
}

func (i *Value) GetOperand(index int) *Value {
	return NewValue(i.GetValues()[index])
}

func (i *Value) GetRawUsers() ssa.Users {
	return i.InstructionNode.GetUsers()
}

func (i *Value) GetUsers() Values {
	return lo.Map(i.GetRawUsers(), func(v ssa.User, _ int) *Value { return NewValue(v) })
}

func (i *Value) GetUser(index int) *Value {
	return NewValue(i.InstructionNode.GetUsers()[index])
}

func (value *Value) ShowUseDefChain() {
	defaultUseDefChain(value).Show()
}

type Values []*Value

func (value Values) Ref(name string) Values {
	// return nil
	var ret Values
	for _, v := range value {
		v.GetRawUsers().RunOnField(func(f *ssa.Field) {
			if f.Key.String() == name {
				ret = append(ret, NewValue(f))
			}
		})
	}
	return ret
}

// func (v Values) UseDefChain(f func(*Value, *UseDefChain)) {
// 	for _, v := range v {
// 		f(v, defaultUseDefChain(v))
// 	}
// }

func (v Values) String() string {
	ret := ""
	ret += fmt.Sprintf("Values: %d\n", len(v))
	for i, v := range v {
		ret += fmt.Sprintf("\t%d: %5s: %s\n", i, v.GetOpcode(), v)
	}
	return ret
}

func (v Values) Show() { fmt.Println(v) }

func (v Values) Get(i int) *Value {
	return v[i]
}

func (v Values) ForEach(f func(*Value)) {
	for _, v := range v {
		f(v)
	}
}

func (v *Value) IsUpdate() bool {
	return v.GetOpcode() == ssa.OpUpdate
}

func (v *Value) IsConst() bool {
	return v.GetOpcode() == ssa.OpConst
}

func (v *Value) IsBinOp() bool {
	return v.GetOpcode() == ssa.BinOpBegin
}
