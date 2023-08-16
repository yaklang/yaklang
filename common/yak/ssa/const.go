package ssa

import (
	"fmt"
	"reflect"
)

var (
	ConstMap = make(map[any]*Const)
)

// create const
func NewConstWithUnary(i any, un int) *Const {
	c := NewConst(i)
	c.Unary = un
	return c
}
func NewConst(i any) *Const {
	// build new const
	typestr := reflect.TypeOf(i).String()
	if typestr == "int" {
		i = int64(i.(int))
		typestr = "int64"
	}
	// after update i
	if c, ok := ConstMap[i]; ok {
		return c
	}
	c := &Const{
		user:  make([]User, 0),
		value: i,
		typ:   Types{basicTypesStr[typestr]},
		str:   fmt.Sprintf("%v", i),
	}
	// const should same
	// assert newConst(1) ==newConst(1)
	ConstMap[i] = c
	return c
}
