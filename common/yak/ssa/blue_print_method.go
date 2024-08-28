package ssa

import (
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
	"golang.org/x/exp/slices"
)

// constructor and destructor
func (c *ClassBluePrint) GetConstructOrDestruct(name string) Value {
	var val Value = nil
	c.getFieldWithParent(func(bluePrint *ClassBluePrint) bool {
		switch name {
		case "constructor":
			if utils.IsNil(bluePrint.Constructor) {
				return false
			}
			val = bluePrint.Constructor
		case "destructor":
			if utils.IsNil(bluePrint.Destructor) {
				return false
			}
			val = bluePrint.Destructor
		}
		return true
	})
	return val
}

// magic
func (c *ClassBluePrint) IsMagicMethodName(name string) bool {
	return slices.Contains(c._container.GetProgram().magicMethodName, name)
}

func (c *ClassBluePrint) RegisterMagicMethod(name string, val *Function) {
	if !c.IsMagicMethodName(name) {
		log.Warnf("register magic method fail: not magic method")
		return
	}
	c.MagicMethod[name] = val
}
func (c *ClassBluePrint) GetMagicMethod(name string) Value {
	var _method Value
	c.getFieldWithParent(func(bluePrint *ClassBluePrint) bool {
		if value := bluePrint.MagicMethod[name]; utils.IsNil(value) {
			return false
		} else {
			_method = value
			return true
		}
	})
	return _method
}

// normal method
func (c *ClassBluePrint) RegisterNormalMethod(name string, val *Function) {
	// if c._container != nil {
	// 	// set the container ref key to the method
	// 	log.Infof("bind %v.%v to function: %v", c.Name, key, fun.name)
	// 	funcContainsklass := c._container.GetFunc()
	// 	if funcContainsklass != nil && funcContainsklass.builder != nil {
	// 		builder := funcContainsklass.builder
	// 		variable := builder.CreateMemberCallVariable(c._container, builder.EmitConstInst(key))
	// 		builder.AssignVariable(variable, fun)
	// 	} else {
	// 		log.Warnf("bind %v.%v failed, reason: class's builder (from source is missed)", c.Name, key)
	// 	}
	// } else {
	// 	log.Warnf("class %v's ref container is nil", c.Name)
	// }
	if f, ok := ToFunction(val); ok {
		f.SetMethod(true, c)
	}
	// if overwrite parent-class/interface, then new function  point to the older
	if f, ok := c.NormalMethod[name]; ok {
		Point(val, f)
	}
	c.NormalMethod[name] = val
}

func (c *ClassBluePrint) GetNormalMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *ClassBluePrint) bool {
		if function, ok := bluePrint.NormalMethod[key]; ok {
			f = function
			return true
		}
		return false
	})
	return f
}

// static method
func (c *ClassBluePrint) RegisterStaticMethod(name string, val Value) {
	c.StaticMethod[name] = val
}

func (c *ClassBluePrint) GetStaticMethod(key string) Value {
	var f Value
	c.getFieldWithParent(func(bluePrint *ClassBluePrint) bool {
		if function, ok := bluePrint.StaticMethod[key]; ok {
			f = function
			return true
		}
		return false
	})
	return f
}

func (c *ClassBluePrint) FinishClassFunction() {
	// lo.ForEach(c.ParentClass, func(item *ClassBluePrint, index int) {
	// 	item.FinishClassFunction()
	// })
	// syntaxHandler := func(functions ...map[string]*Function) {
	// 	lo.ForEach(functions, func(item map[string]*Function, index int) {
	// 		for _, value := range item {
	// 			function, ok := ToFunction(value)
	// 			if !ok {
	// 				continue
	// 			}
	// 			function.Build()
	// 			function.FixSpinUdChain()
	// 		}
	// 	})
	// }
	// checkAndGetMaps := func(vals ...Value) map[string]*Function {
	// 	var results = make(map[string]*Function)
	// 	lo.ForEach(vals, func(item Value, index int) {
	// 		if funcs, b := ToFunction(c.Constructor); b {
	// 			results[uuid.NewString()] = funcs
	// 		}
	// 	})
	// 	return results
	// }
	// syntaxHandler(c.StaticMethod, c.NormalMethod, checkAndGetMaps(c.Constructor, c.Destructor))
}
