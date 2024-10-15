package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

// normal member
func (c *BluePrint) RegisterNormalMember(name string, val Value) {
	c.storeInContainer(name, val, BluePrintNormalMember)
	c.NormalMember[name] = val
}
func (c *BluePrint) RegisterNormalConst(name string, val Value) {
	c.storeInContainer(name, val, BluePrintConstMember)
	c.ConstValue[name] = val
}

func (c *BluePrint) GetNormalMember(name string) Value {
	var member Value
	c.getFieldWithParent(func(bluePrint *BluePrint) bool {
		if value, ok := bluePrint.NormalMember[name]; ok {
			member = value
			return true
		}
		return false
	})
	return member
}

// static member
func (c *BluePrint) RegisterStaticMember(name string, val Value) {
	c.storeInContainer(name, val, BluePrintStaticMember)
	c.StaticMember[name] = val
}

func (c *BluePrint) GetStaticMember(name string) Value {
	var member Value
	c.getFieldWithParent(func(bluePrint *BluePrint) bool {
		if value := bluePrint.StaticMember[name]; !utils.IsNil(value) {
			member = value
			return true
		}
		return false
	})
	return member
}

// const member
func (c *BluePrint) RegisterConstMember(name string, val Value) {
	c.ConstValue[name] = val
}
func (c *BluePrint) GetConstMember(key string) Value {
	var val Value
	c.getFieldWithParent(func(bluePrint *BluePrint) bool {
		if value, ok := bluePrint.ConstValue[key]; ok {
			val = value
			return true
		}
		return false
	})
	return val
}
