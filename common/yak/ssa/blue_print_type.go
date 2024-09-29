package ssa

import (
	"fmt"
)

var _ Type = (*ClassBluePrint)(nil)

/// ============= implement type interface

func (c *ClassBluePrint) String() string {
	str := fmt.Sprintf("ClassBluePrint: %s", c.Name)
	return str
}

func (c *ClassBluePrint) PkgPathString() string {
	return ""
}

func (c *ClassBluePrint) RawString() string {
	return ""
}

func (c *ClassBluePrint) GetTypeKind() TypeKind {
	return ClassBluePrintTypeKind
}

func (c *ClassBluePrint) SetMethod(m map[string]*Function) {
	c.NormalMethod = m
}

func (c *ClassBluePrint) AddMethod(key string, fun *Function) {
	c.RegisterNormalMethod(key, fun)
}

func (c *ClassBluePrint) GetMethod() map[string]*Function {
	return c.NormalMethod
}

func (c *ClassBluePrint) SetMethodGetter(f func() map[string]*Function) {
}

func (c *ClassBluePrint) AddFullTypeName(name string) {
	if c == nil {
		return
	}
	c.fullTypeName = append(c.fullTypeName, name)
}

func (c *ClassBluePrint) GetFullTypeNames() []string {
	if c == nil {
		return nil
	}
	return c.fullTypeName
}

func (c *ClassBluePrint) SetFullTypeNames(names []string) {
	if c == nil {
		return
	}
	c.fullTypeName = names
}
