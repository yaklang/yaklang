package ssaapi

import (
	"fmt"

	"github.com/yaklang/yaklang/common/yak/ssa"
)

type Values struct {
	ns []ssa.Node
}

func NewValue(n []ssa.Node) *Values {
	return &Values{
		ns: n,
	}
}

func (value *Values) Ref(name string) *Values {
	// return nil
	var ret []ssa.Node
	for _, v := range value.ns {
		v.GetUsers().RunOnField(func(f *ssa.Field) {
			if f.Key.String() == name {
				ret = append(ret, f)
			}
		})
	}
	return NewValue(ret)
}

func (v *Values) UseDefChain(f func(*UseDefChain)) {
	// ret := make(UseDefChains, 0, len(v.ns))
	for _, v := range v.ns {
		// ret = append(ret, defaultUseDefChain(v))
		f(defaultUseDefChain(v))
	}
	// return ret
}

func (v *Values) Show() {
	ret := ""
	ret += fmt.Sprintf("Value: %d\n", len(v.ns))
	for i, v := range v.ns {
		ret += fmt.Sprintf("  %d: %s\n", i, v.String())
	}
	fmt.Println(ret)
}

type Direction int

const (
	Both Direction = iota
	Use
	UseBy
)

type UseDefChain struct {
	direction Direction // 1 is up; -1 is down; 0 is both
	v         ssa.Node
}

func defaultUseDefChain(v ssa.Node) *UseDefChain {
	return &UseDefChain{
		direction: Both,
		v:         v,
	}
}

func (u *UseDefChain) Show() {
	ret := "use def chain\n"
	v := u.v
	for _, v := range v.GetValues() {
		ret += fmt.Sprintf("\tUse  \t%s\n", v)
	}

	ret += fmt.Sprintf("\tSelf\t%s\n", v)
	for _, u := range v.GetUsers() {
		ret += fmt.Sprintf("\tUseBy\t%s\n", u)
	}

	fmt.Println(ret)
}

func (u *UseDefChain) SetDirectionUse() *UseDefChain {
	u.direction = Use
	return u
}

func (u *UseDefChain) SetSetDirectionUseBy() *UseDefChain {
	u.direction = UseBy
	return u
}

func (u *UseDefChain) Walk(f func(*Instruction)) {
	v := u.v
	if u.direction == Both || u.direction == Use {
		// walk values
		for _, v := range v.GetValues() {
			f(NewInstruction(v))
		}
	}

	if u.direction == Both || u.direction == UseBy {
		// walk  users
		for _, u := range v.GetUsers() {
			f(NewInstruction(u))
		}
	}
}
