package ssa

// normal member
func (c *Blueprint) RegisterNormalMember(name string, val Value, store ...bool) {
	c.NormalMember[name] = append(c.NormalMember[name], val)
	if len(store) == 0 || store[0] {
		c.storeField(name, val, BluePrintNormalMember)
	}
}
func (c *Blueprint) RegisterNormalConst(name string, val Value, store ...bool) {
	c.storeField(name, val, BluePrintConstMember)
	c.ConstValue[name] = append(c.ConstValue[name], val)
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
		}
		return false
	})
	return members
}

// static member
func (c *Blueprint) RegisterStaticMember(name string, val Value, store ...bool) {
	c.StaticMember[name] = append(c.StaticMember[name], val)
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
		}
		return false
	})
	return members
}

// const member
func (c *Blueprint) RegisterConstMember(name string, val Value) {
	c.ConstValue[name] = append(c.ConstValue[name], val)
	c.storeField(name, val, BluePrintConstMember)
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
