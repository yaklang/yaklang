package ssa

import "fmt"

type ClassMethod struct {
	*Function
	This  Value
	Index int // index of function parameter
}

func NewClassMethod(fun *Function, this Value, index int) *ClassMethod {
	return &ClassMethod{
		Function: fun,
		This:     this,
		Index:    index,
	}
}

var _ Value = (*ClassMethod)(nil)

func (c *ClassMethod) String() string {
	str := fmt.Sprintf("ClassMethod: %s", c.Function.GetName())
	return str
}
