package ssa

import (
	"fmt"
	"github.com/google/uuid"
	"github.com/samber/lo"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

type BluePrintMagicMethodKind string

const (
	Constructor BluePrintMagicMethodKind = "constructor"
	Destructor                           = "destructor"
)

// magic
func (c *BluePrint) IsMagicMethodName(name BluePrintMagicMethodKind) bool {
	return slices.Contains(c._container.GetProgram().magicMethodName, string(name))
}

func (c *BluePrint) RegisterMagicMethod(name BluePrintMagicMethodKind, val *Function) {
	if !c.IsMagicMethodName(name) {
		log.Warnf("register magic method fail: not magic method")
		return
	}
	c.MagicMethod[name] = val
}
func (c *BluePrint) GetMagicMethod(name BluePrintMagicMethodKind) Value {
	var _method Value
	c.getFieldWithParent(func(bluePrint *BluePrint) bool {
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
func (c *BluePrint) RegisterNormalMethod(name string, val *Function, store ...bool) {
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

func (c *BluePrint) GetNormalMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *BluePrint) bool {
		if function, ok := bluePrint.NormalMethod[key]; ok {
			f = function
			return true
		}
		return false
	})
	return f
}

// static method
func (c *BluePrint) RegisterStaticMethod(name string, val *Function) {
	c.StaticMethod[name] = val
}

func (c *BluePrint) GetStaticMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *BluePrint) bool {
		if function, ok := bluePrint.StaticMethod[key]; ok {
			f = function
			return true
		}
		return false
	})
	return f
}

func (c *BluePrint) FinishClassFunction() {
	lo.ForEach(c.ParentClass, func(item *BluePrint, index int) {
		item.FinishClassFunction()
	})
	syntaxHandler := func(functions ...map[string]*Function) {
		lo.ForEach(functions, func(item map[string]*Function, index int) {
			for _, value := range item {
				function, ok := ToFunction(value)
				if !ok {
					continue
				}
				function.Build()
				function.FixSpinUdChain()
			}
		})
	}
	checkAndGetMaps := func(vals ...Value) map[string]*Function {
		var results = make(map[string]*Function)
		lo.ForEach(vals, func(item Value, index int) {
			if funcs, b := ToFunction(c.Constructor); b {
				results[uuid.NewString()] = funcs
			}
		})
		return results
	}
	syntaxHandler(c.StaticMethod, c.NormalMethod, checkAndGetMaps(c.Constructor, c.Destructor))
}
