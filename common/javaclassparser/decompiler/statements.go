package decompiler

import (
	"fmt"
	"strings"
)

type Statement interface {
	String(funcCtx *FunctionContext) string
}
type ConditionStatement struct {
	Condition   JavaValue
	Op          string
	ToStatement int
}

func (r *ConditionStatement) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("if %s goto %d", r.Condition.String(funcCtx), r.ToStatement)
}

func NewConditionStatement(cmp JavaValue, op string) *ConditionStatement {
	if v, ok := cmp.(*JavaCompare); ok {
		return &ConditionStatement{
			Condition: NewBinaryExpression(v.JavaValue1, v.JavaValue2, op),
			Op:        op,
		}
	} else {
		return &ConditionStatement{
			Condition: cmp,
			Op:        op,
		}
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

type StackAssignStatement struct {
	Id        int
	JavaValue JavaValue
}

func (a *StackAssignStatement) String(funcCtx *FunctionContext) string {
	return a.JavaValue.String(funcCtx)
}
func NewStackAssignStatement(id int, value JavaValue) *StackAssignStatement {
	return &StackAssignStatement{
		Id:        id,
		JavaValue: value,
	}
}

type AssignStatement struct {
	LeftValue   JavaValue
	ArrayMember *JavaArrayMember
	JavaValue   JavaValue
	IsFirst     bool
}

func (a *AssignStatement) String(funcCtx *FunctionContext) string {
	if a.ArrayMember != nil {
		return fmt.Sprintf("%s = %s", a.ArrayMember.String(funcCtx), a.JavaValue.String(funcCtx))
	}
	assign := fmt.Sprintf("%s = %s", a.LeftValue.String(funcCtx), a.JavaValue.String(funcCtx))
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
	datas = append(datas, fmt.Sprintf("%s %s %s", f.Condition.String(funcCtx)))
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

func NewArrayMemberAssignStatement(m *JavaArrayMember, value JavaValue) *AssignStatement {
	return &AssignStatement{
		ArrayMember: m,
		JavaValue:   value,
	}
}

func NewAssignStatement(leftVal, value JavaValue, isFirst bool) *AssignStatement {
	return &AssignStatement{
		LeftValue: leftVal,
		JavaValue: value,
		IsFirst:   isFirst,
	}
}

type IfStatement struct {
	Condition JavaValue
	IfBody    []Statement
	ElseBody  []Statement
}

func (g *IfStatement) String(funcCtx *FunctionContext) string {
	getBody := func(sts []Statement) string {
		var res []string
		for _, st := range sts {
			res = append(res, st.String(funcCtx))
		}
		return strings.Join(res, "\n")
	}
	return fmt.Sprintf("if (%s){\n"+
		"%s\n"+
		"}else{\n"+
		"%s\n"+
		"}", g.Condition.String(funcCtx), getBody(g.IfBody), getBody(g.ElseBody))
}
func NewIfStatement(condition JavaValue, ifBody, elseBody []Statement) *IfStatement {
	return &IfStatement{
		Condition: condition,
		IfBody:    ifBody,
		ElseBody:  elseBody,
	}
}

type GOTOStatement struct {
	ToStatement int
}

func (g *GOTOStatement) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("goto: %d", g.ToStatement)
}
func NewGOTOStatement() *GOTOStatement {
	return &GOTOStatement{}
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

type ExpressionStatement struct {
	Expression JavaValue
}

func (a *ExpressionStatement) String(funcCtx *FunctionContext) string {
	return a.Expression.String(funcCtx)
}

func NewExpressionStatement(v JavaValue) *ExpressionStatement {
	return &ExpressionStatement{
		Expression: v,
	}
}

type CaseItem struct {
	IsDefault bool
	IntValue  int
	Body      []Statement
}

func NewCaseItem(v int, body []Statement) *CaseItem {
	return &CaseItem{
		Body:     body,
		IntValue: v,
	}
}

type SwitchStatement struct {
	Value JavaValue
	Cases []*CaseItem
}

func (a *SwitchStatement) String(funcCtx *FunctionContext) string {
	casesStrs := []string{}
	for _, c := range a.Cases {
		if c.IsDefault {
			casesStrs = append(casesStrs, fmt.Sprintf("default:\n%s", StatementsString(c.Body, funcCtx)))
			continue
		}
		casesStrs = append(casesStrs, fmt.Sprintf("case %d:\n%s", c.IntValue, StatementsString(c.Body, funcCtx)))
	}
	return fmt.Sprintf("switch(%s) {\n%s\n}", a.Value.String(funcCtx), strings.Join(casesStrs, "\n"))
}

func NewSwitchStatement(value JavaValue, cases []*CaseItem) *SwitchStatement {
	return &SwitchStatement{
		Value: value,
		Cases: cases,
	}
}

const (
	MiddleSwitch = "switch"
)

type MiddleStatement struct {
	Data any
	Flag string
}

func (a *MiddleStatement) String(funcCtx *FunctionContext) string {
	return "<middle statement>"
}

func NewMiddleStatement(flag string, d any) *MiddleStatement {
	return &MiddleStatement{
		Flag: flag,
		Data: d,
	}
}

type SynchronizedStatement struct {
	Argument JavaValue
	Body     []Statement
}

func NewSynchronizedStatement(val JavaValue, body []Statement) *SynchronizedStatement {
	return &SynchronizedStatement{Argument: val, Body: body}
}

func (s *SynchronizedStatement) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("synchronized(%s) {\n%s\n}", s.Argument.String(funcCtx), StatementsString(s.Body, funcCtx))
}
