package ssa

import (
	"fmt"

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

func (c *Blueprint) RegisterMagicMethod(name BlueprintMagicMethodKind, val Value) {
	if !c.IsMagicMethodName(name) {
		log.Warnf("register magic method fail: not magic method")
		//return
	}
	if method, exit := c.MagicMethod[name]; exit {
		Point(val, method)
	} else {
		c.MagicMethod[name] = val
	}
	switch name {
	case Constructor:
		c.Constructor = val
		c.storeField(c.Name, val, BluePrintMagicMethod)
		val.SetVerboseName(c.Name)
		return
	case Destructor:
		c.Destructor = val
	}
	c.storeField(val.GetName(), val, BluePrintMagicMethod)
}

func (c *Blueprint) GetMagicMethod(name BlueprintMagicMethodKind, fb *FunctionBuilder) Value {
	var _method Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		switch name {
		case Constructor:
			if utils.IsNil(bluePrint.Constructor) {
				return false
			} else {
				//check blueprint is virtual
				_func := bluePrint._container.GetFunc()
				if _func.name == string(VirtualFunctionName) {
					return false
				}
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
		if c.Range != nil {
			backup := fb.CurrentRange
			fb.CurrentRange = c.Range
			defer func() {
				fb.CurrentRange = backup
			}()
		}

		switch name {
		case Constructor:
			_name := fmt.Sprintf("%s-constructor", c.Name)
			//constructor := c.GeneralUndefined(_name)
			constructor := fb.EmitUndefined(_name)
			_method = constructor
			functionType := NewFunctionType(fmt.Sprintf("%s-%s", c.Name, string(name)), []Type{c}, c, true)
			functionType.SetFullTypeNames(c.GetFullTypeNames())
			_method.SetType(functionType)
			c.RegisterMagicMethod(Constructor, _method)
		case Destructor:
			_name := fmt.Sprintf("%s-destructor", c.Name)
			destructor := fb.EmitUndefined(_name)
			_method = destructor
			functionType := NewFunctionType(fmt.Sprintf("%s-%s", c.Name, string(name)), []Type{c}, c, true)
			functionType.SetFullTypeNames(c.GetFullTypeNames())
			_method.SetType(functionType)
			c.RegisterMagicMethod(Destructor, _method)
		default:
			return nil
		}
	}
	if !utils.IsNil(_method) {
		if function, b := ToFunction(_method); b {
			_ = function
			function.Build()
		}
	}
	return _method
}

// normal method
func (c *Blueprint) RegisterNormalMethod(name string, val *Function, store ...bool) {
	if len(store) == 0 || store[0] == true {
		c.storeField(name, val, BluePrintNormalMethod)
	}
	if f, ok := ToFunction(val); ok {
		f.SetMethod(true, c)
	}
	if method := c.NormalMethod[name]; !utils.IsNil(method) {
		Point(method, val)
	} else {
		c.NormalMethod[name] = val
	}
}

func (c *Blueprint) GetNormalMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if function, ok := bluePrint.NormalMethod[key]; ok {
			f = function
			function.Build()
			return true
		}
		return false
	})
	return f
}

// static method
func (c *Blueprint) RegisterStaticMethod(name string, val *Function) {
	if method := c.StaticMethod[name]; !utils.IsNil(method) {
		Point(method, val)
	} else {
		c.storeField(name, val, BluePrintStaticMember)
		c.StaticMethod[name] = val
	}
}

func (c *Blueprint) GetStaticMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if function, ok := bluePrint.StaticMethod[key]; ok {
			f = function
			function.Build()
			return true
		}
		return false
	})
	return f
}
