package ssa

import "github.com/yaklang/yaklang/common/utils"

func sameBlueprintMemberValue(left, right Value) bool {
	if utils.IsNil(left) || utils.IsNil(right) {
		return utils.IsNil(left) && utils.IsNil(right)
	}
	return left.GetId() == right.GetId()
}

func appendBlueprintMember(members map[string][]Value, name string, val Value) bool {
	if utils.IsNil(val) {
		return false
	}
	values := members[name]
	if len(values) > 0 && sameBlueprintMemberValue(values[len(values)-1], val) {
		return false
	}
	members[name] = append(values, val)
	return true
}

// normal member
func (c *Blueprint) RegisterNormalMember(name string, val Value, store ...bool) {
	if !appendBlueprintMember(c.NormalMember, name, val) {
		return
	}
	if len(store) == 0 || store[0] {
		c.storeField(name, val, BluePrintNormalMember)
	}
}
func (c *Blueprint) RegisterNormalConst(name string, val Value, store ...bool) {
	if !appendBlueprintMember(c.ConstValue, name, val) {
		return
	}
	if len(store) == 0 || store[0] {
		c.storeField(name, val, BluePrintConstMember)
	}
}

func (c *Blueprint) GetNormalMember(name string) Value {
	var member Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if values, ok := bluePrint.NormalMember[name]; ok && len(values) > 0 {
			member = values[len(values)-1]
			return true
		}
		return false
	})
	return member
}

func (c *Blueprint) GetNormalMembers(name string) []Value {
	var members []Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if values, ok := bluePrint.NormalMember[name]; ok && len(values) > 0 {
			members = append(members, values...)
			return true
		}
		return false
	})
	return members
}

// static member
func (c *Blueprint) RegisterStaticMember(name string, val Value, store ...bool) {
	if !appendBlueprintMember(c.StaticMember, name, val) {
		return
	}
	if len(store) == 0 || store[0] {
		c.storeField(name, val, BluePrintStaticMember)
	}
}

func (c *Blueprint) GetStaticMember(name string) Value {
	var member Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if values := bluePrint.StaticMember[name]; len(values) > 0 {
			member = values[len(values)-1]
			return true
		}
		return false
	})
	return member
}

func (c *Blueprint) GetStaticMembers(name string) []Value {
	var members []Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if values := bluePrint.StaticMember[name]; len(values) > 0 {
			members = append(members, values...)
			return true
		}
		return false
	})
	return members
}

// const member
func (c *Blueprint) RegisterConstMember(name string, val Value, store ...bool) {
	if !appendBlueprintMember(c.ConstValue, name, val) {
		return
	}
	if len(store) == 0 || store[0] {
		c.storeField(name, val, BluePrintConstMember)
	}
}
func (c *Blueprint) GetConstMember(key string) Value {
	var val Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if values, ok := bluePrint.ConstValue[key]; ok && len(values) > 0 {
			val = values[len(values)-1]
			return true
		}
		return false
	})
	return val
}

func (class *Blueprint) Read(name string) []Value {
	if val := class.GetStaticMembers(name); len(val) > 0 {
		return append([]Value(nil), val...)
	}
	if normalMember := class.GetNormalMembers(name); len(normalMember) > 0 {
		return append([]Value(nil), normalMember...)
	}
	if method := class.GetNormalMethod(name); method != nil {
		return []Value{method}
	}
	return nil
}
