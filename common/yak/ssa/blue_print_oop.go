package ssa

import (
	"fmt"

	"github.com/yaklang/yaklang/common/log"
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

func (pkg *Package) GetClassBluePrint(name string) *ClassBluePrint {
	if c, ok := pkg.ClassBluePrint[name]; ok {
		return c
	}
	// log.Errorf("GetClassBluePrint: not this class: %s", name)
	return nil
}

func (b *FunctionBuilder) SetClassBluePrint(name string, class *ClassBluePrint) {
	p := b.Package
	if _, ok := p.ClassBluePrint[name]; ok {
		log.Errorf("SetClassBluePrint: this class redeclare")
	}
	p.ClassBluePrint[name] = class
}

func (b *FunctionBuilder) CreateClassBluePrint(name string) *ClassBluePrint {
	// p := b.GetProgram()
	p := b.Package
	c := NewClassBluePrint()
	if _, ok := p.ClassBluePrint[name]; ok {
		log.Errorf("CreateClassBluePrint: this class redeclare")
	}
	p.ClassBluePrint[name] = c
	c.Name = name
	return c
}

func (b *FunctionBuilder) GetClassBluePrint(name string) *ClassBluePrint {
	// p := b.GetProgram()
	p := b.Package
	return p.GetClassBluePrint(name)
}

func (b *FunctionBuilder) GetStaticMember(class, key string) *Variable {
	return b.CreateVariable(fmt.Sprintf("%s_%s", class, key))
}

func (c *ClassBluePrint) GetMember(key string, get func(*ClassBluePrint) (Value, bool)) Value {
	if v, ok := get(c); ok {
		return v
	}
	for _, p := range c.ParentClass {
		if v := p.GetMember(key, get); v != nil {
			return v
		}
	}
	log.Errorf("VisitClassMember: this class: %s no this member %s", c.String(), key)
	return nil
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
	c.NormalMember[name] = BluePrintMember{
		Value: value,
		Type:  value.GetType(),
	}
}

func (c *ClassBluePrint) AddNormalMemberOnlyType(name string, typ Type) {
	c.NormalMember[name] = BluePrintMember{
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
	c.ParentClass = append(c.ParentClass, parent)
}
