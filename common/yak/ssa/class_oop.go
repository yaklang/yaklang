package ssa

import "github.com/yaklang/yaklang/common/log"

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

func (b *FunctionBuilder) GetClass(name string) *ClassBluePrint {
	if c, ok := b.ClassBluePrint[name]; ok {
		return c
	}
	log.Errorf("VisitClass: not this class: %s", name)
	return nil
}

func (b *FunctionBuilder) GetStaticMember(class, key string) Value {
	c := b.GetClass(class)
	if c == nil {
		return nil
	}
	return c.GetMember(key, func(class *ClassBluePrint) (Value, bool) {
		v, ok := class.StaticMember[key]
		return v, ok
	})
}

// func (b *FunctionBuilder) GetNormalMember(class, key string) Value {
// 	c := b.GetClass(class)
// 	if c == nil {
// 		log.Errorf("VisitStaticClass: not this class: %s", class)
// 		return nil
// 	}
// 	return c.GetMember(key, func(cbp *ClassBluePrint) (Value, bool) {
// 		v, ok := cbp.NormalMember[key]
// 		return v, ok
// 	})
// }

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

func (b *FunctionBuilder) GetClassConstructor(class string) Value {
	c := b.GetClass(class)
	if c == nil {
		log.Errorf("VisitStaticClass: not this class: %s", class)
		return nil
	}
	return c.Constructor
}

func (b *FunctionBuilder) SetStaticMember(class, key string, value Value) {
	c := b.GetClass(class)
	if c == nil {
		log.Errorf("VisitStaticClass: not this class: %s", class)
		c = b.CreateClass(class)
	}

	c.BuildStaticMember(key, value)
}

func (b *FunctionBuilder) CreateClass(name string) *ClassBluePrint {
	c := NewClassBluePrint()
	if _, ok := b.ClassBluePrint[name]; ok {
		log.Errorf("CreateClass: this class redeclare")
	}
	b.ClassBluePrint[name] = c
	c.Name = name
	return c
}

//======================= class blue print

func (c *ClassBluePrint) BuildMember(name string, value Value) {
	c.NormalMember[name] = value
}

func (c *ClassBluePrint) BuildStaticMember(name string, value Value) {
	c.StaticMember[name] = value
}
