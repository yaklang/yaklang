package decompiler

import (
	"fmt"
	"strings"
)

type Statement interface {
	String(funcCtx *FunctionContext) string
}
type ConditionStatement struct {
	RightValue  JavaValue
	LeftValue   JavaValue
	Op          string
	ToOpcode    int
	ToStatement int
}

func (r *ConditionStatement) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("if %s %s %s goto %d", r.LeftValue.String(funcCtx), r.Op, r.RightValue.String(funcCtx), r.ToStatement)
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

func (r *ReturnStatement) String(funcCtx *FunctionContext) string {
	if r.JavaValue == nil {
		return "return"
	}

	return fmt.Sprintf("return %s", r.JavaValue.String(funcCtx))
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

func (f *FunctionCallStatement) String(funcCtx *FunctionContext) string {
	paramStrs := []string{}
	for _, param := range f.Params {
		paramStrs = append(paramStrs, param.String(funcCtx))
	}
	return fmt.Sprintf("%s.%s(%s)", f.Object.String(funcCtx), f.FunctionName, strings.Join(paramStrs, ","))
}

func NewFunctionCallStatement(object JavaValue, name string, params []JavaValue) *FunctionCallStatement {
	return &FunctionCallStatement{
		Object:       object,
		FunctionName: name,
		Params:       params,
	}
}

type DeclareStatement struct {
	Id       int
	JavaType JavaType
}

func (a *DeclareStatement) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("%s var%d", a.JavaType.String(funcCtx), a.Id)
}

func NewDeclareStatement(id int, typ JavaType) *DeclareStatement {
	return &DeclareStatement{
		Id:       id,
		JavaType: typ,
	}
}

type AssignStatement struct {
	Id        int
	JavaValue JavaValue
	IsFirst   bool
}

func (a *AssignStatement) String(funcCtx *FunctionContext) string {
	assign := fmt.Sprintf("var%d = %s", a.Id, a.JavaValue.String(funcCtx))
	if a.IsFirst {
		return a.JavaValue.Type().String(funcCtx) + " " + assign
	} else {
		return assign
	}
}

type ForStatement struct {
	InitVar             Statement
	Condition           *ConditionStatement
	EndExp              Statement
	SubStatements       []Statement
	SubStatementsDumper func(subStatement Statement) string
}

func NewForStatement(subStatements []Statement) *ForStatement {
	return &ForStatement{
		InitVar:       subStatements[0],
		Condition:     subStatements[1].(*ConditionStatement),
		EndExp:        subStatements[len(subStatements)-2],
		SubStatements: subStatements[2 : len(subStatements)-2],
	}
}
func (f *ForStatement) String(funcCtx *FunctionContext) string {
	datas := []string{}
	datas = append(datas, f.InitVar.String(funcCtx))
	datas = append(datas, fmt.Sprintf("%s %s %s", f.Condition.LeftValue.String(funcCtx), f.Condition.Op, f.Condition.RightValue.String(funcCtx)))
	datas = append(datas, f.EndExp.String(funcCtx))
	statementStr := []string{}
	for _, statement := range f.SubStatements {
		if f.SubStatementsDumper != nil {
			statementStr = append(statementStr, f.SubStatementsDumper(statement))
		} else {
			statementStr = append(statementStr, statement.String(funcCtx))
		}
	}
	s := fmt.Sprintf("for(%s; %s; %s) {\n%s\n}", datas[0], datas[1], datas[2], strings.Join(statementStr, "\n"))
	return s
}
func NewAssignStatement(id int, value JavaValue, isFirst bool) *AssignStatement {
	return &AssignStatement{
		Id:        id,
		JavaValue: value,
		IsFirst:   isFirst,
	}
}

type GOTOStatement struct {
	ToOpcode    int
	ToStatement int
}

func (g *GOTOStatement) String(funcCtx *FunctionContext) string {
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

func (a *NewStatement) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("new %s()", a.Class.Name)
}

func NewNewStatement(class *JavaClass) *NewStatement {
	return &NewStatement{
		Class: class,
	}
}
