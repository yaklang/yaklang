package ssa

import (
	"fmt"

	"github.com/samber/lo"
)

var _ Type = (*Blueprint)(nil)

/// ============= implement type interface

func (c *Blueprint) String() string {
	str := fmt.Sprintf("ClassBluePrint: %s", c.Name)
	return str
}

func (c *Blueprint) PkgPathString() string {
	return ""
}

func (c *Blueprint) RawString() string {
	return ""
}

func (c *Blueprint) GetTypeKind() TypeKind {
	return ClassBluePrintTypeKind
}

func (c *Blueprint) SetMethod(m map[string]*Function) {
	c.NormalMethod = m
}

func (c *Blueprint) AddMethod(key string, fun *Function) {
	c.RegisterNormalMethod(key, fun)
}

func (c *Blueprint) GetMethod() map[string]*Function {
	return c.NormalMethod
}

func (c *Blueprint) SetMethodGetter(f func() map[string]*Function) {
}

func (c *Blueprint) AddFullTypeName(name string) {
	if c == nil {
		return
	}
	if !lo.Contains(c.fullTypeName, name) {
		c.fullTypeName = append(c.fullTypeName, name)
	}
}

func (c *Blueprint) GetFullTypeNames() []string {
	if c == nil {
		return nil
	}
	return c.fullTypeName
}

func (c *Blueprint) SetFullTypeNames(names []string) {
	if c == nil {
		return
	}
	c.fullTypeName = clean(names)
}
