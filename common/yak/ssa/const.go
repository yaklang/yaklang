package ssa

import (
	"fmt"
	"sync"
)

type Const struct {
	value any
	// only one type
	typ Type
	str string
}

// get type
func (c *Const) GetType() Type {
	t := c.typ
	if t == nil {
		t = BasicTypes[Any]
	}
	return t
}

func (c *Const) SetType(ts Type) {
	// const don't need set type
}

var (
	ConstMap      = make(map[any]*Const)
	ConstMapMutex = &sync.RWMutex{}
)

func init() {
	// ConstMapMutex.Lock()
	// defer ConstMapMutex.Unlock()

	ConstMap[nil] = &Const{
		value: nil,
		typ:   BasicTypes[Null],
		str:   "nil",
	}

	ConstMap[struct{}{}] = &Const{
		value: struct{}{},
		typ:   BasicTypes[Any],
		str:   "any",
	}
}

func NewNil() *ConstInst {
	return NewConst(nil)
}

func NewAny() *ConstInst {
	return NewConst(struct{}{})
}

// create const
func NewConstWithUnary(i any, un int) *ConstInst {
	c := NewConst(i)
	c.Unary = un
	return c
}

func NewConst(i any) *ConstInst {
	c := newConstByMap(i)
	if c == nil {
		c = newConstCreate(i)
	}
	ci := &ConstInst{
		Const:         c,
		anInstruction: NewInstruction(),
		anValue:       NewValue(),
		Unary:         0,
	}
	return ci
}

func newConstCreate(i any) *Const {
	// build new const
	typ := GetType(i)
	c := &Const{
		value: i,
		typ:   typ,
		str:   fmt.Sprintf("%v", i),
	}
	ConstMapMutex.Lock()
	ConstMap[i] = c
	ConstMapMutex.Unlock()
	return c
}

func newConstByMap(i any) *Const {
	// after update i
	ConstMapMutex.RLock()
	defer ConstMapMutex.RUnlock()
	c, ok := ConstMap[i]
	if ok {
		return c
	} else {
		return nil
	}
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
