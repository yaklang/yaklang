package ssa

import (
	"fmt"
)

var (
	ConstMap = make(map[any]*Const)

	UnDefineConst = &Const{
		user:  []User{},
		value: nil,
		typ:   []Type{BasicTypesKind[Undefine]},
		str:   "Undefine",
		Unary: 0,
	}
)

// create const
func NewConstWithUnary(i any, un int) *Const {
	c := NewConst(i)
	c.Unary = un
	return c
}
func NewConst(i any) *Const {
	// build new const
	typ := GetType(i)
	// after update i
	if c, ok := ConstMap[i]; ok {
		return c
	}
	c := &Const{
		user:  make([]User, 0),
		value: i,
		typ:   Types{typ},
		str:   fmt.Sprintf("%v", i),
	}
	// const should same
	// assert newConst(1) ==newConst(1)
	ConstMap[i] = c
	return c
}
