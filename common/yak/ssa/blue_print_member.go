package ssa

import (
	"github.com/yaklang/yaklang/common/utils"
)

// normal member
func (b *Blueprint) RegisterNormalMember(name string, val Value, store ...bool) {
	b.NormalMember[name] = val
	if len(store) == 0 || store[0] {
		b.storeField(name, val, BluePrintNormalMember)
	}
}
func (b *Blueprint) RegisterNormalConst(name string, val Value, store ...bool) {
	b.storeField(name, val, BluePrintConstMember)
	b.ConstValue[name] = val
}

func (b *Blueprint) GetNormalMember(name string) Value {
	var member Value
	b.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if value, ok := bluePrint.NormalMember[name]; ok {
			member = value
			return true
		}
		return false
	})
	return member
}

// static member
func (b *Blueprint) RegisterStaticMember(name string, val Value, store ...bool) {
	b.StaticMember[name] = val
	if len(store) == 0 || store[0] {
		b.storeField(name, val, BluePrintStaticMember)
	}
}

func (b *Blueprint) GetStaticMember(name string) Value {
	var member Value
	b.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if value := bluePrint.StaticMember[name]; !utils.IsNil(value) {
			member = value
			return true
		}
		return false
	})
	return member
}

// const member
func (b *Blueprint) RegisterConstMember(name string, val Value) {
	b.ConstValue[name] = val
	b.storeField(name, val, BluePrintConstMember)
}
func (b *Blueprint) GetConstMember(key string) Value {
	var val Value
	b.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if value, ok := bluePrint.ConstValue[key]; ok {
			val = value
			return true
		}
		return false
	})
	return val
}
