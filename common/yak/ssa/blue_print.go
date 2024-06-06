package ssa

import (
	"fmt"
)

type method struct {
	function *Function
	index    int
}

type BluePrintMember struct {
	Value Value
	Type  Type
}

// ClassBluePrint is a class blue print, it is used to create a new class
type ClassBluePrint struct {
	Name string

	Method       map[string]*Function
	StaticMethod map[string]*Function

	NormalMember map[string]BluePrintMember
	StaticMember map[string]Value

	CallBack []func()

	// magic method
	Copy        Value
	Constructor Value
	Destructor  Value

	ParentClass []*ClassBluePrint
}

func NewClassBluePrint() *ClassBluePrint {
	class := &ClassBluePrint{
		NormalMember: make(map[string]BluePrintMember),
		StaticMember: make(map[string]Value),

		Method:       make(map[string]*Function),
		StaticMethod: make(map[string]*Function),
	}

	return class
}

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
	c.Method = m
}
func (c *ClassBluePrint) AddMethod(key string, fun *Function) {
	c.Method[key] = fun
}
func (c *ClassBluePrint) GetMethod() map[string]*Function {
	return c.Method
}
