package ssa

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

type BlueprintMagicMethodKind string

const (
	Constructor BlueprintMagicMethodKind = "constructor"
	Destructor                           = "destructor"
)

// magic
func (c *Blueprint) IsMagicMethodName(name BlueprintMagicMethodKind) bool {
	return slices.Contains(c._container.GetProgram().magicMethodName, string(name))
}

func (c *Blueprint) RegisterMagicMethod(name BlueprintMagicMethodKind, val *Function) {
	if !c.IsMagicMethodName(name) {
		log.Warnf("register magic method fail: not magic method")
		return
	}
	c.MagicMethod[name] = val
}
func (c *Blueprint) GetMagicMethod(name BlueprintMagicMethodKind) Value {
	var _method Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		switch name {
		case Constructor:
			if utils.IsNil(bluePrint.Constructor) {
				return false
			} else {
				_method = bluePrint.Constructor
				return true
			}
		case Destructor:
			if utils.IsNil(bluePrint.Destructor) {
				return false
			} else {
				_method = bluePrint.Constructor
				return true
			}
		default:
			if value := bluePrint.MagicMethod[name]; utils.IsNil(value) {
				return false
			} else {
				_method = value
				return true
			}
		}
	})
	if utils.IsNil(_method) {
		switch name {
		case Constructor:
			_name := fmt.Sprintf("%s-constructor", c.Name)
			constructor := c.GeneralUndefined(_name)
			_method = constructor
			_method.SetType(NewFunctionType(fmt.Sprintf("%s-%s", c.Name, string(name)), []Type{c}, c, true))
		case Destructor:
			_name := fmt.Sprintf("%s-destructor", c.Name)
			destructor := c.GeneralUndefined(_name)
			_method = destructor
			_method.SetType(NewFunctionType(fmt.Sprintf("%s-%s", c.Name, string(name)), []Type{c}, c, true))
		default:
			return nil
		}
	}
	return _method
}

// normal method
func (c *Blueprint) RegisterNormalMethod(name string, val *Function, store ...bool) {
	if len(store) == 0 || store[0] == true {
		c.storeInContainer(name, val, BluePrintNormalMethod)
	}
	if f, ok := ToFunction(val); ok {
		f.SetMethod(true, c)
	}
	// if overwrite parent-class/interface, then new function  point to the older
	if method := c.GetNormalMethod(name); !utils.IsNil(method) {
		Point(val, method)
	}
	c.NormalMethod[name] = val
}

func (c *Blueprint) GetNormalMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if function, ok := bluePrint.NormalMethod[key]; ok {
			f = function
			return true
		}
		return false
	})
	return f
}

// static method
func (c *Blueprint) RegisterStaticMethod(name string, val *Function) {
	c.storeInContainer(name, val, BluePrintStaticMember)
	c.StaticMethod[name] = val
}

func (c *Blueprint) GetStaticMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if function, ok := bluePrint.StaticMethod[key]; ok {
			f = function
			return true
		}
		return false
	})
	return f
}
