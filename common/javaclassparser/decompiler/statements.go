package decompiler

import (
	"fmt"
	"strings"
)

type Statement interface {
	String() string
}
type ConditionStatement struct {
	RightValue  JavaValue
	LeftValue   JavaValue
	Op          string
	ToOpcode    int
	ToStatement int
}

func (r *ConditionStatement) String() string {
	return fmt.Sprintf("if %s %s %s goto %d", r.LeftValue.String(), r.Op, r.RightValue.String(), r.ToStatement)
}

func NewConditionStatement(l, r JavaValue, op string, to int) *ConditionStatement {
	return &ConditionStatement{
		LeftValue:  l,
		RightValue: r,
		Op:         op,
		ToOpcode:   to,
	}
}

type ReturnStatement struct {
	JavaValue JavaValue
}

func (r *ReturnStatement) String() string {
	if r.JavaValue == nil {
		return "return;"
	}

	return fmt.Sprintf("return %s;", r.JavaValue.String())
}

func NewReturnStatement(value JavaValue) *ReturnStatement {
	return &ReturnStatement{
		JavaValue: value,
	}
}

type FunctionCallStatement struct {
	Object       JavaValue
	FunctionName string
	Params       []JavaValue
}

func (f *FunctionCallStatement) String() string {
	paramStrs := []string{}
	for _, param := range f.Params {
		paramStrs = append(paramStrs, param.String())
	}
	return fmt.Sprintf("%s.%s(%s)", f.Object.String(), f.FunctionName, strings.Join(paramStrs, ","))
}

func NewFunctionCallStatement(object JavaValue, name string, params []JavaValue) *FunctionCallStatement {
	return &FunctionCallStatement{
		Object:       object,
		FunctionName: name,
		Params:       params,
	}
}

type AssignStatement struct {
	Id        int
	JavaValue JavaValue
}

func (a *AssignStatement) String() string {
	return fmt.Sprintf("var%d = %s", a.Id, a.JavaValue.String())
}

func NewAssignStatement(id int, value JavaValue) *AssignStatement {
	return &AssignStatement{
		Id:        id,
		JavaValue: value,
	}
}

type GOTOStatement struct {
	ToOpcode    int
	ToStatement int
}

func (g *GOTOStatement) String() string {
	return fmt.Sprintf("goto: %d", g.ToStatement)
}
func NewGOTOStatement(target int) *GOTOStatement {
	return &GOTOStatement{
		ToOpcode: target,
	}
}

type NewStatement struct {
	Class *JavaClass
}

func (a *NewStatement) String() string {
	return fmt.Sprintf("new %s();", a.Class.Name)
}

func NewNewStatement(class *JavaClass) *NewStatement {
	return &NewStatement{
		Class: class,
	}
}
