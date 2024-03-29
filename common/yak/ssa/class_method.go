package ssa

import "fmt"

type ClassMethod struct {
	anValue
	Func *Function
	This Value
}

func NewClassMethod(fun *Function, this Value) *ClassMethod {
	return &ClassMethod{
		anValue: NewValue(),
		Func:    fun,
		This:    this,
	}
}

func (f *ClassMethod) HasValues() bool   { return false }
func (f *ClassMethod) GetValues() Values { return nil }

var _ Value = (*ClassMethod)(nil)

func (c *ClassMethod) String() string {
	str := fmt.Sprintf("ClassMethod: %s", c.Func.GetName())
	return str
}
