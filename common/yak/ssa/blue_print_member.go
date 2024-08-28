package ssa

import "github.com/yaklang/yaklang/common/utils"

// normal member
func (c *ClassBluePrint) RegisterNormalMember(name string, val Value) {
	// val.GetProgram().SetInstructionWithName(name, val)
	c.NormalMember[name] = val
}

func (c *ClassBluePrint) GetNormalMember(name string) Value {
	var member Value
	c.getFieldWithParent(func(bluePrint *ClassBluePrint) bool {
		if value, ok := bluePrint.NormalMember[name]; ok {
			member = value
			return true
		}
		return false
	})
	return member
}

// static member
func (c *ClassBluePrint) RegisterStaticMember(name string, val Value) {
	phi, ok := c.StaticMember[name]
	if !ok {
		phi = c.GeneralPhi(name)
		c.StaticMember[name] = phi
	}
	phi.Edge = append(phi.Edge, val)
}

func (c *ClassBluePrint) GetStaticMember(name string) Value {
	var member Value
	c.getFieldWithParent(func(bluePrint *ClassBluePrint) bool {
		if value := bluePrint.StaticMember[name]; !utils.IsNil(value) {
			member = value
			return true
		}
		return false
	})
	return member
}

// const member
func (c *ClassBluePrint) RegisterConstMember(name string, val Value) {
	c.ConstValue[name] = val
}
func (c *ClassBluePrint) GetConstMember(key string) Value {
	var val Value
	c.getFieldWithParent(func(bluePrint *ClassBluePrint) bool {
		if value, ok := bluePrint.ConstValue[key]; ok {
			val = value
			return true
		}
		return false
	})
	return val
}
