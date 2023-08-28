package ssa

import (
	"fmt"
)

var (
	ConstMap = make(map[any]*Const)

	UnDefineConst = &Const{
		user:  []User{},
		value: nil,
		typ:   []Type{BasicTypesKind[UndefineType]},
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

func (c *Const) IsBoolean() bool {
	return c.typ[0] == BasicTypesKind[Boolean]
}

func (c *Const) Boolean() bool {
	return c.value.(bool)
}

func (c *Const) IsNumber() bool {
	return c.typ[0] == BasicTypesKind[Number]
}

func (c *Const) Number() int64 {
	switch ret := c.value.(type) {
	case int:
		return int64(ret)
	case int8:
		return int64(ret)
	case int16:
		return int64(ret)
	case int32:
		return int64(ret)
	case int64:
		return ret
	case uint:
		return int64(ret)
	case uint8:
		return int64(ret)
	case uint16:
		return int64(ret)
	case uint32:
		return int64(ret)
	case uint64:
		return int64(ret)
	}
	return 0
}

func (c *Const) IsFloat() bool {
	return c.typ[0] == BasicTypesKind[Number]
}

func (c *Const) Float() float64 {
	switch ret := c.value.(type) {
	case float32:
		return float64(ret)
	case float64:
		return ret
	}
	return float64(c.Number())
}

func (c *Const) IsString() bool {
	return c.typ[0] == BasicTypesKind[String]
}

func (c *Const) VarString() string {
	return c.value.(string)
}
