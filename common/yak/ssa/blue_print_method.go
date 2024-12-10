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
		//return
	}
	functions, ok := c.MagicMethod[name]
	if !ok {
		c.MagicMethod[name] = append(c.MagicMethod[name], val)
	} else {
		if method := functions.GetFunctionByHash(val.hash); method != nil {
			Point(val, method)
		} else {
			c.MagicMethod[name] = append(c.MagicMethod[name], val)
		}
	}
	switch name {
	case Constructor:
		c.Constructor = val
		c.storeInContainer(c.Name, val, BluePrintMagicMethod)
		val.SetVerboseName(c.Name)
		return
	case Destructor:
		c.Destructor = val
	}
	c.storeInContainer(val.GetName(), val, BluePrintMagicMethod)
}

func (c *Blueprint) GetMagicMethod(name BlueprintMagicMethodKind, process ...FunctionProcess) Value {
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
			if functions, ok := bluePrint.MagicMethod[name]; ok {
				if method := functions.GetFunctionByProcess(process); method != nil {
					_method = method
					return true
				}
			}
			return false
		}
	})
	if utils.IsNil(_method) {
		switch name {
		case Constructor:
			_name := fmt.Sprintf("%s-constructor", c.Name)
			constructor := c.GenerateFunction(_name)
			constructor.ParamsType = []Type{c}
			constructor.ParamLength = 1
			constructor.SetType(NewFunctionType(fmt.Sprintf("%s-%s", c.Name, string(name)), []Type{c}, c, true))
			_method = constructor
			c.RegisterMagicMethod(Constructor, constructor)
		case Destructor:
			_name := fmt.Sprintf("%s-destructor", c.Name)
			destructor := c.GenerateFunction(_name)
			destructor.ParamsType = []Type{c}
			destructor.ParamLength = 1
			destructor.SetType(NewFunctionType(fmt.Sprintf("%s-%s", c.Name, string(name)), []Type{c}, c, true))
			_method = destructor
			c.RegisterMagicMethod(Destructor, destructor)
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
		c.storeInContainer(name, val, BluePrintNormalMethod)
	}
	if f, ok := ToFunction(val); ok {
		f.SetMethod(true, c)
	}
	functions, ok := c.NormalMethod[name]
	if !ok {
		c.NormalMethod[name] = append(c.NormalMethod[name], val)
		return
	}
	if method := functions.GetFunctionByHash(val.hash); method != nil {
		Point(method, val)
	} else {
		c.NormalMethod[name] = append(c.NormalMethod[name], val)
	}
}

func (c *Blueprint) GetNormalMethod(key string, process ...FunctionProcess) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if functions, ok := bluePrint.NormalMethod[key]; ok {
			function := functions.GetFunctionByProcess(process)
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
	methods, ok := c.StaticMethod[name]
	if !ok {
		c.storeInContainer(name, val, BluePrintStaticMember)
		c.StaticMethod[name] = append(c.StaticMethod[name], val)
	}
	if method := methods.GetFunctionByHash(val.hash); method != nil {
		Point(method, val)
	} else {
		c.storeInContainer(name, val, BluePrintStaticMember)
		c.StaticMethod[name] = append(c.StaticMethod[name], val)
	}
}

func (c *Blueprint) GetStaticMethod(key string, process ...FunctionProcess) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *Blueprint) bool {
		if functions, ok := bluePrint.StaticMethod[key]; ok {
			function := functions.GetFunctionByProcess(process)
			f = function
			function.Build()
			return true
		}
		return false
	})
	return f
}
