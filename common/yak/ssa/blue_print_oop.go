package ssa

import (
	"fmt"
	"github.com/samber/lo"
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

func (p *Program) GetClassBlueprintEx(name string, pkg string) *ClassBluePrint {
	if p == nil {
		return nil
	}

	//todo: this have error,refactor class pr after fix
	m := p.importTypeToPkg[name]
	if m != nil && len(m) > 0 {
		entries := lo.Entries(m)
		if bluePrint, ok := entries[0].Value.(*ClassBluePrint); ok {
			return bluePrint
		}
	}
	if p.importType[pkg] != nil {
		if _type, ok := p.importType[pkg][name]; ok {
			if bluePrint, ok1 := _type.(*ClassBluePrint); ok1 {
				if !p.PreHandler() {
					bluePrint.Build()
				}
				return bluePrint
			}
		}
	}
	if c, ok := p.ClassBluePrint[name]; ok {
		if !p.PreHandler() {
			c.Build()
		}
		return c
	}
	return nil
}
func (p *Program) GetClassBluePrint(name string) *ClassBluePrint {
	return p.GetClassBlueprintEx(name, "")
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
func (b *FunctionBuilder) CreateBluePrintWithPkgName(name string, tokenizer ...CanStartStopToken) *ClassBluePrint {
	prog := b.prog
	blueprint := NewClassBluePrint()
	if prog.ClassBluePrint == nil {
		prog.ClassBluePrint = make(map[string]*ClassBluePrint)
	}
	blueprint.Name = name
	prog.ClassBluePrint[name] = blueprint
	klassvar := b.CreateVariable(name, tokenizer...)
	klassContainer := b.EmitEmptyContainer()
	b.AssignVariable(klassvar, klassContainer)
	if err := blueprint.InitializeWithContainer(klassContainer); err != nil {
		log.Errorf("CreateClassBluePrint.InitializeWithContainer error: %s", err)
	}
	return blueprint
}
func (b *FunctionBuilder) CreateClassBluePrint(name string, tokenizer ...CanStartStopToken) *ClassBluePrint {
	return b.CreateBluePrintWithPkgName(name, tokenizer...)
}

func (b *FunctionBuilder) GetClassBluePrint(name string) *ClassBluePrint {
	// p := b.GetProgram()
	p := b.prog
	return p.GetClassBluePrint(name)
}

func (c *ClassBluePrint) GetMemberAndStaticMember(key string, supportStatic bool) Value {
	var member Value
	c.GetMemberEx(key, func(c *ClassBluePrint) bool {
		if m, ok := c.NormalMember[key]; ok {
			member = m.Value
			return true
		}
		if supportStatic {
			if value, ok := c.StaticMember[key]; ok {
				member = value
				return true
			}
		}
		return false
	})
	return member
}

func (c *ClassBluePrint) GetConstructOrDestruct(name string) Value {
	var val Value = nil
	c.getMethodEx("", func(bluePrint *ClassBluePrint) bool {
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
func (c *ClassBluePrint) GetConstEx(key string, get func(c *ClassBluePrint) bool) bool {
	if b := get(c); b {
		return true
	} else {
		for _, class := range c.ParentClass {
			if ex := class.GetConstEx(key, get); ex {
				return true
			}
		}
	}
	return false
}
func (c *ClassBluePrint) GetMethodAndStaticMethod(name string, supportStatic bool) *Function {
	var _func *Function
	c.getMethodEx(name, func(bluePrint *ClassBluePrint) bool {
		if function, ok := bluePrint.Method[name]; ok {
			_func = function
			return true
		} else if supportStatic {
			if f, ok := bluePrint.StaticMethod[name]; ok {
				_func = f
				return true
			}
		}
		return false
	})
	return _func
}
func (c *ClassBluePrint) getMethodEx(name string, get func(bluePrint *ClassBluePrint) bool) bool {
	if b := get(c); b {
		return true
	}
	for _, class := range c.ParentClass {
		if ex := class.getMethodEx(name, get); ex {
			return true
		}
	}
	return false
}

func (b *FunctionBuilder) GetStaticMember(class, key string) *Variable {
	_ = b.GetProgram().GetClassBluePrint(class)
	return b.CreateVariable(fmt.Sprintf("%s_%s", class, key))
}

func (c *ClassBluePrint) GetMemberEx(key string, get func(*ClassBluePrint) bool) bool {
	if ok := get(c); ok {
		return true
	}
	for _, p := range c.ParentClass {
		if ok := p.GetMemberEx(key, get); ok {
			return true
		}
	}
	// log.Errorf("VisitClassMember: this class: %s no this member %s", c.String(), key)
	return false
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

// AddNormalMember is used to add a normal member to the class,
func (c *ClassBluePrint) AddNormalMember(name string, value Value) {
	value.GetProgram().SetInstructionWithName(name, value)
	c.NormalMember[name] = &BluePrintMember{
		Value: value,
		Type:  value.GetType(),
	}
}

func (c *ClassBluePrint) AddNormalMemberOnlyType(name string, typ Type) {
	c.NormalMember[name] = &BluePrintMember{
		Value: nil,
		Type:  typ,
	}
}

// AddStaticMember is used to add a static member to the class,
func (c *ClassBluePrint) AddStaticMember(name string, value Value) {
	c.StaticMember[name] = value
}

// AddStaticMethod is used to add a static method to the class,
func (c *ClassBluePrint) AddStaticMethod(name string, value *Function) {
	c.StaticMethod[name] = value
}

// AddParentClass is used to add a parent class to the class,
func (c *ClassBluePrint) AddParentClass(parent *ClassBluePrint) {
	if parent == nil {
		return
	}
	c.ParentClass = append(c.ParentClass, parent)
	for name, f := range parent.Method {
		c.Method[name] = f
	}
}
