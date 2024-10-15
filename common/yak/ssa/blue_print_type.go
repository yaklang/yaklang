package ssa

import (
	"fmt"
	"github.com/samber/lo"
)

var _ Type = (*BluePrint)(nil)

/// ============= implement type interface

func (c *BluePrint) String() string {
	str := fmt.Sprintf("ClassBluePrint: %s", c.Name)
	return str
}

func (c *BluePrint) PkgPathString() string {
	return ""
}

func (c *BluePrint) RawString() string {
	return ""
}

func (c *BluePrint) GetTypeKind() TypeKind {
	return ClassBluePrintTypeKind
}

func (c *BluePrint) SetMethod(m map[string]*Function) {
	c.NormalMethod = m
}

func (c *BluePrint) AddMethod(key string, fun *Function) {
	c.RegisterNormalMethod(key, fun)
}

func (c *BluePrint) GetMethod() map[string]*Function {
	return c.NormalMethod
}

func (c *BluePrint) SetMethodGetter(f func() map[string]*Function) {
}

func (c *BluePrint) AddFullTypeName(name string) {
	if c == nil {
		return
	}
	if !lo.Contains(c.fullTypeName, name) {
		c.fullTypeName = append(c.fullTypeName, name)
	}
}

func (c *BluePrint) GetFullTypeNames() []string {
	if c == nil {
		return nil
	}
	return c.fullTypeName
}

func (c *BluePrint) SetFullTypeNames(names []string) {
	if c == nil {
		return
	}
	c.fullTypeName = names
}
