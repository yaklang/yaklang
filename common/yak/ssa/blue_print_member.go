package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

// normal member
func (c *Blueprint) RegisterNormalMember(name string, val Value) {
	c.storeInContainer(name, val, BluePrintNormalMember)
	c.NormalMember[name] = val
}
func (c *Blueprint) RegisterNormalConst(name string, val Value) {
	c.storeInContainer(name, val, BluePrintConstMember)
	c.ConstValue[name] = val
}

func (c *Blueprint) GetNormalMember(name string) Value {
	var member Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if value, ok := bluePrint.NormalMember[name]; ok {
			member = value
			return true
		}
		return false
	})
	return member
}

// static member
func (c *Blueprint) RegisterStaticMember(name string, val Value) {
	c.storeInContainer(name, val, BluePrintStaticMember)
	c.StaticMember[name] = val
}

func (c *Blueprint) GetStaticMember(name string) Value {
	var member Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if value := bluePrint.StaticMember[name]; !utils.IsNil(value) {
			member = value
			return true
		}
		return false
	})
	return member
}

// const member
func (c *Blueprint) RegisterConstMember(name string, val Value) {
	c.ConstValue[name] = val
}
func (c *Blueprint) GetConstMember(key string) Value {
	var val Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if value, ok := bluePrint.ConstValue[key]; ok {
			val = value
			return true
		}
		return false
	})
	return val
}
