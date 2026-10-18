package ssa

import "github.com/yaklang/yaklang/common/utils"

func sameBlueprintMemberValue(left, right Value) bool {
	if utils.IsNil(left) || utils.IsNil(right) {
		return utils.IsNil(left) && utils.IsNil(right)
	}
	return left.GetId() == right.GetId()
}

func appendBlueprintMember(members *utils.SafeMapWithKey[string, []Value], name string, val Value) bool {
	if members == nil || utils.IsNil(val) {
		return false
	}
	added := false
	members.Update(name, func(old []Value, loaded bool) ([]Value, bool) {
		if loaded && len(old) > 0 && sameBlueprintMemberValue(old[len(old)-1], val) {
			return old, false
		}
		// Copy so a reader still holding the previous slice does not share
		// the backing array with this append.
		next := make([]Value, len(old)+1)
		copy(next, old)
		next[len(old)] = val
		added = true
		return next, true
	})
	return added
}

func blueprintMemberValues(members *utils.SafeMapWithKey[string, []Value], name string) ([]Value, bool) {
	if members == nil {
		return nil, false
	}
	return members.Get(name)
}

// inheritBlueprintMembers copies a parent's member list onto a child that
// does not already have that name. Register runs outside the source map lock.
func inheritBlueprintMembers(dst, src *utils.SafeMapWithKey[string, []Value], register func(string, Value)) {
	if dst == nil || src == nil || register == nil {
		return
	}
	src.ForEach(func(name string, values []Value) bool {
		if existing, ok := dst.Get(name); ok && len(existing) > 0 {
			return true
		}
		for _, value := range values {
			register(name, value)
		}
		return true
	})
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
		if values, ok := blueprintMemberValues(bluePrint.NormalMember, name); ok && len(values) > 0 {
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
		if values, ok := blueprintMemberValues(bluePrint.NormalMember, name); ok && len(values) > 0 {
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
		if values, _ := blueprintMemberValues(bluePrint.StaticMember, name); len(values) > 0 {
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
		if values, _ := blueprintMemberValues(bluePrint.StaticMember, name); len(values) > 0 {
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
		if values, ok := blueprintMemberValues(bluePrint.ConstValue, key); ok && len(values) > 0 {
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
