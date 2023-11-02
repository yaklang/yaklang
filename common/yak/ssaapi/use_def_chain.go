package ssaapi

import (
	"fmt"
	"strconv"
)

type Direction int

const (
	Both Direction = iota
	Use
	UseBy
)

type UseDefChain struct {
	direction Direction
	v         *Value
}

func defaultUseDefChain(v *Value) *UseDefChain {
	return &UseDefChain{
		direction: Both,
		v:         v,
	}
}

func (u *UseDefChain) Show() {
	u.ShowEx(0)
}

func (u *UseDefChain) ShowAll() {
	u.ShowEx(1)
}

func (u *UseDefChain) ShowEx(flag int) {
	v := u.v
	ret := fmt.Sprintf("use def chain [%s]:\n", v.node.GetOpcode())

	show := func(prefix string, index int, v *Value) string {
		indexStr := ""
		if index >= 0 {
			indexStr = strconv.FormatInt(int64(index), 10)
		}
		ret := ""
		switch flag {
		case 0:
			ret += fmt.Sprintf("\t%s\t%s\t%s\n", prefix, indexStr, v)
		case 1:
			ret += fmt.Sprintf("\t%s\t%s\n\t\t%s\n\t\t%s\n\t\t%s\n", prefix, indexStr, v, v.node, v.node.GetPosition())
		default:
		}
		return ret
	}

	for i, v := range v.GetOperands() {
		ret += show("Operand", i, v)
	}

	ret += show("Self", -1, v)
	for i, u := range v.GetUsers() {
		ret += show("User", i, u)
	}
	fmt.Println(ret)
}

func (u *UseDefChain) WalkUser(f func(*Value)) {
	// walk  users
	v := u.v
	for _, u := range v.GetUsers() {
		f(u)
	}
}

func (u *UseDefChain) WalkOperand(f func(*Value)) {
	v := u.v
	for _, v := range v.GetOperands() {
		f(v)
	}
}
