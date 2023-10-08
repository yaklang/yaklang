package ssa

import (
	"fmt"
)

func init() {
	ConstMap[nil] = &Const{
		anNode: NewNode(),
		value:  nil,
		typ:    BasicTypes[Null],
		str:    "nil",
		Unary:  0,
	}

	ConstMap[struct{}{}] = &Const{
		anNode: anNode{},
		value:  struct{}{},
		typ:    BasicTypes[Any],
		str:    "any",
		Unary:  0,
	}
}

func NewNil() *Const {
	return ConstMap[nil]
}

func NewAny() *Const {
	return ConstMap[struct{}{}]
}

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
	// after update i
	if c, ok := ConstMap[i]; ok {
		return c
	}
	// build new const
	typ := GetType(i)
	c := &Const{
		anNode: NewNode(),
		value:  i,
		typ:    typ,
		str:    fmt.Sprintf("%v", i),
	}
	// const should same
	// assert newConst(1) ==newConst(1)
	ConstMap[i] = c
	return c
}

func (c *Const) GetTypeKind() TypeKind {
	return c.typ.GetTypeKind()
}

func (c *Const) IsBoolean() bool {
	return c.typ.GetTypeKind() == Boolean
}

func (c *Const) Boolean() bool {
	return c.value.(bool)
}

func (c *Const) IsNumber() bool {
	return c.typ.GetTypeKind() == Number
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
	return c.typ.GetTypeKind() == Number
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
	return c.typ.GetTypeKind() == String
}

func (c *Const) VarString() string {
	return c.value.(string)
}
