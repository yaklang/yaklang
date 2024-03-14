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
	return nil
}

func (b *FunctionBuilder) GetStaticMember(class, key string) Value {
	c := b.GetClass(class)
	if c == nil {
		log.Errorf("VisitStaticClass: not this class: %s", class)
		return nil
	}
	v := c.GetStaticMember(key)
	if v == nil {
		log.Errorf("VisitStaticClass: this class: %s no this member %s", class, key)
		return nil
	}
	return v
}

func (b *FunctionBuilder) SetStaticMember(class, key string, value Value) {
	c := b.GetClass(class)
	if c == nil {
		log.Errorf("VisitStaticClass: not this class: %s", class)
	}

	c.BuildStaticMember(key, value)
}

func (b *FunctionBuilder) CreateClass(name string) *ClassBluePrint {
	c := NewClassBluePrint()
	if _, ok := b.ClassBluePrint[name]; ok {
		log.Errorf("CreateClass: this class redeclare")
	}
	b.ClassBluePrint[name] = c
	return c
}

func (c *ClassBluePrint) BuildMember(name string, value Value) {
	c.NormalMember[name] = value
}

func (c *ClassBluePrint) BuildStaticMember(name string, value Value) {
	c.StaticMember[name] = value
}

// ============= class member
func (c *ClassBluePrint) GetStaticMember(member string) Value {
	if v, ok := c.StaticMember[member]; ok {
		return v
	}
	return nil
}
