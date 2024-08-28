package ssa

import (
	"fmt"
	"github.com/yaklang/yaklang/common/log"
	"github.com/yaklang/yaklang/common/utils"
)

type ClassModifier int

const (
	NoneModifier ClassModifier = 1 << iota
	Static
	Public
	Protected
	Private
	Abstract
	Final
	Readonly
)

func (pkg *Program) GetClassBluePrint(name string) *ClassBluePrint {
	if pkg == nil {
		return nil
	}
	if c, ok := pkg.ClassBluePrint[name]; ok {
		return c
	}
	// log.Errorf("GetClassBluePrint: not this class: %s", name)
	return nil
}

func (b *FunctionBuilder) SetClassBluePrint(name string, class *ClassBluePrint) {
	p := b.prog
	if _, ok := p.ClassBluePrint[name]; ok {
		log.Errorf("SetClassBluePrint: this class redeclare")
	}
	p.ClassBluePrint[name] = class
}

// CreateClassBluePrint will create object template (maybe class)
// in dynamic and classless language, we can create object without class
// but because of the 'this/super', we will still keep the concept 'Class'
// for ref the method/function, the blueprint is a container too,
// saving the static variables and util methods.
func (b *FunctionBuilder) CreateClassBluePrint(name string, tokenizer ...CanStartStopToken) *ClassBluePrint {
	// p := b.GetProgram()
	p := b.prog
	c := NewClassBluePrint()
	if _, ok := p.ClassBluePrint[name]; ok {
		log.Errorf("CreateClassBluePrint: this class redeclare")
	}
	p.ClassBluePrint[name] = c
	c.Name = name

	log.Infof("start to create class container variable for saving static member: %s", name)
	klassVar := b.CreateVariable(name, tokenizer...)
	klassContainer := b.EmitEmptyContainer()
	b.AssignVariable(klassVar, klassContainer)
	err := c.InitializeWithContainer(klassContainer)
	members, _ := b.GetProgram().GetApplication().GlobalScope.GetStringMember("$staticScope$")
	builder := b.mainBuilder
	variable := builder.CreateMemberCallVariable(members, builder.EmitConstInst(name))
	container := builder.EmitEmptyContainer()
	builder.AssignVariable(variable, container)
	c.InitStaticContainer(container)
	if err != nil {
		log.Errorf("CreateClassBluePrint.InitializeWithContainer error: %s", err)
	}
	return c
}
func (c *ClassBluePrint) GetMagicMethod(name string) Value {
	var _method Value
	c.getKlassEx(name, func(bluePrint *ClassBluePrint) bool {
		if value := bluePrint.magicMethod[name]; utils.IsNil(value) {
			return false
		} else {
			_method = value
			return true
		}
	})
	return _method
}
func (c *ClassBluePrint) GetStaticMember(name string) Value {
	var member Value
	c.getKlassEx(name, func(bluePrint *ClassBluePrint) bool {
		if value := bluePrint.StaticMember[name]; !utils.IsNil(value) {
			member = value
			return true
		}
		return false
	})
	return member
}

// GetNormalMember todo: this read seem error
func (c *ClassBluePrint) GetNormalMember(name string) Value {
	var member Value
	c.getKlassEx(name, func(bluePrint *ClassBluePrint) bool {
		if printMember, ok := bluePrint.NormalMember[name]; ok {
			member = printMember.Value
			return true
		}
		return false
	})
	return member
}

func (b *FunctionBuilder) GetClassBluePrint(name string) *ClassBluePrint {
	// p := b.GetProgram()
	p := b.prog
	return p.GetClassBluePrint(name)
}

func (c *ClassBluePrint) GetConstructOrDestruct(name string) Value {
	var val Value = nil
	c.getKlassEx("", func(bluePrint *ClassBluePrint) bool {
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
func (c *ClassBluePrint) GetConst(key string) Value {
	var val Value
	c.getKlassEx(key, func(bluePrint *ClassBluePrint) bool {
		if value, ok := bluePrint.ConstValue[key]; ok {
			val = value
			return true
		}
		return false
	})
	return val
}
func (c *ClassBluePrint) GetStaticMethod(key string) *Function {
	var f *Function
	c.getKlassEx(key, func(bluePrint *ClassBluePrint) bool {
		if function, ok := bluePrint.StaticMethod[key]; ok {
			f = function
			return true
		}
		return false
	})
	return f
}

func (c *ClassBluePrint) GetMethod_(key string) *Function {
	var f *Function
	c.getKlassEx(key, func(bluePrint *ClassBluePrint) bool {
		if function, ok := bluePrint.Method[key]; ok {
			f = function
			return true
		}
		return false
	})
	return f
}

// CreateStaticMemberVariable need global builder assign
func (c *ClassBluePrint) CreateStaticMemberVariable(key string) *Variable {
	builder := c._staticContainer.GetFunc().builder
	return builder.CreateMemberCallVariable(c._staticContainer, builder.EmitConstInst(key))
}
func (c *ClassBluePrint) getKlassEx(key string, get func(bluePrint *ClassBluePrint) bool) bool {
	if b := get(c); b {
		return true
	} else {
		for _, class := range c.ParentClass {
			if ex := class.getKlassEx(key, get); ex {
				return true
			}
		}
	}
	return false
}

func (b *FunctionBuilder) CreateStaticMember(class, key string) *Variable {
	member, _ := b.mainBuilder.GetProgram().GlobalScope.GetStringMember("$staticScope$")
	if stringMember, ok := member.GetStringMember(class); ok {
		return b.mainBuilder.CreateMemberCallVariable(stringMember, b.mainBuilder.EmitConstInst(key))
	}
	log.Errorf("not found this class in global scope")
	return b.mainBuilder.CreateVariable(fmt.Sprintf("%s_%s", class, key))
}
func (b *FunctionBuilder) ReadClsStaticMember(class, key string) Value {
	member_ := b.mainBuilder.CreateVariable(fmt.Sprintf("%s_%s", class, key))
	if val := b.PeekValueByVariable(member_); !utils.IsNil(val) {
		return val
	}
	staticMembers := b.CreateStaticMember(class, key)
	if val := b.mainBuilder.PeekValueByVariable(staticMembers); !utils.IsNil(val) {
		return val
	}
	return b.EmitUndefined(fmt.Sprintf("$%s$%s$", class, key))
}

//======================= class blue print

// AddNormalMethod is used to add a method to the class,
// parameters: name, function of the method, function, index of the this object in parameter
// func (c *ClassBluePrint) AddNormalMethod(name string, fun *Function, index int) {
// 	c.NormalMethod[name] = &method{
// 		function: fun,
// 		index:    index,
// 	}
// }

func (c *ClassBluePrint) AddNormalMemberOnlyType(name string, typ Type) {
	c.NormalMember[name] = &BluePrintMember{
		Value: nil,
		Type:  typ,
	}
}

// AddStaticMember is used to add a static member to the class,
//func (c *ClassBluePrint) AddStaticMember(name string, value Value) {
//	c.StaticMember[name] = value
//}
//
//// AddStaticMethod is used to add a static method to the class,
//func (c *ClassBluePrint) AddStaticMethod(name string, value *Function) {
//	c.StaticMethod[name] = value
//}
//

// AddParentClass is used to add a parent class to the class,
func (c *ClassBluePrint) AddParentClass(parent *ClassBluePrint) {
	if parent == nil {
		return
	}
	c.ParentClass = append(c.ParentClass, parent)
	for name, f := range parent.Method {
		c.RegisterNormalMethod(name, f)
	}
	for s, value := range parent.StaticMember {
		c.RegisterStaticMember(s, value)
		delete(c.StaticMember, s)
	}
	for s, value := range parent.ConstValue {
		c.RegisterConstMember(s, value)
	}
}
