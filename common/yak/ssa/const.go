package ssa

import (
	"fmt"
	"sync"

	"github.com/yaklang/yaklang/common/utils"
)

type Const struct {
	value any
	// only one type
	// typ Type
	str string
}

func (c *Const) GetRawValue() any {
	if c.value == nil {
		return nil
	}
	return c.value
}

// // get type
// func (c *Const) GetType() Type {
// 	t := c.typ
// 	if t == nil {
// 		t = CreateAnyType()
// 	}
// 	return t
// }

// func (c *Const) SetType(ts Type) {
// 	// const don't need set type
// }

var (
	ConstMap      = make(map[any]*Const)
	ConstMapMutex = &sync.RWMutex{}
)

func init() {
	// ConstMapMutex.Lock()
	// defer ConstMapMutex.Unlock()

	ConstMap[nil] = &Const{
		value: nil,
		// typ:   CreateNullType(),
		str: "nil",
	}
}

func NewNil() *ConstInst {
	return NewConst(nil)
}

func NewAny() *ConstInst {
	return NewConst("")
}

// create const
func NewConstWithUnary(i any, un int) *ConstInst {
	c := NewConst(i)
	c.Unary = un
	return c
}

func NewConst(i any, isPlaceHolder ...bool) *ConstInst {
	placeHolder := false
	if len(isPlaceHolder) > 0 {
		placeHolder = isPlaceHolder[0]
	}

	c := newConstByMap(i)
	if c == nil {
		c = newConstCreate(i)
	}
	ci := &ConstInst{
		Const:     c,
		anValue:   NewValue(),
		Unary:     0,
		ConstType: ConstTypeNormal,
	}
	if placeHolder {
		ci.ConstType = ConstTypePlaceholder
	}
	ci.SetType(GetType(i))
	return ci
}

func newConstCreate(i any) *Const {
	// build new const
	c := &Const{
		value: i,
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

func (c *ConstInst) GetTypeKind() TypeKind {
	return c.GetType().GetTypeKind()
}

func (c *ConstInst) IsBoolean() bool {
	return c.GetType().GetTypeKind() == BooleanTypeKind
}

func (c *Const) Boolean() bool {
	return c.value.(bool)
}

func (c *Const) IsNumber() bool {
	// utils.ToLowerAndStrip()
	return utils.InterfaceToString(c.Number()) == utils.InterfaceToString(c.value)
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

func (c *ConstInst) IsFloat() bool {
	switch c.value.(type) {
	case float32:
		return c.GetType().GetTypeKind() == NumberTypeKind
	case float64:
		return c.GetType().GetTypeKind() == NumberTypeKind
	}
	return false
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

func (c *ConstInst) IsString() bool {
	return c.GetType().GetTypeKind() == StringTypeKind
}

func (c *Const) VarString() string {
	if c.value == nil {
		return ""
	}
	return fmt.Sprintf("%v", c.value)
}
