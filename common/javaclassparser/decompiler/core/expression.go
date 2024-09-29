package core

import (
	"fmt"
	"strings"
)

const (
	ADD = "add"
	INC = "inc"
	New = "new"
)

type NewExpression struct {
	IsArray bool
	JavaType
}

func NewNewArrayExpression(typ JavaType) *NewExpression {
	return &NewExpression{
		JavaType: typ,
		IsArray:  true,
	}
}
func NewNewExpression(typ JavaType) *NewExpression {
	return &NewExpression{
		JavaType: typ,
	}
}
func (n *NewExpression) Type() JavaType {
	return n.JavaType
}

func (n *NewExpression) String(funcCtx *FunctionContext) string {
	if n.IsArray {
		typ := n.JavaType.(*JavaArrayType)
		s := fmt.Sprintf("new %s", typ.JavaType.String(funcCtx))
		for _, l := range typ.Length {
			s += fmt.Sprintf("[%v]", l.String(funcCtx))
		}
		return s
	}
	return fmt.Sprintf("new %s()", n.JavaType.String(funcCtx))
}

type JavaExpression struct {
	Values []JavaValue
	Op     string
}

func (j *JavaExpression) Type() JavaType {
	return j.Values[0].Type()
}

func (j *JavaExpression) String(funcCtx *FunctionContext) string {
	vs := []string{}
	for _, value := range j.Values {
		vs = append(vs, value.String(funcCtx))
	}
	switch j.Op {
	case ADD:
		return fmt.Sprintf("(%s) + (%s)", vs[0], vs[1])
	case INC:
		return fmt.Sprintf("(%s) += (%s)", vs[0], vs[1])
	case GT, SUB:
		return fmt.Sprintf("(%s) %s (%s)", vs[0], j.Op, vs[1])
	default:
		return fmt.Sprintf("(%s) %s (%s)", vs[0], j.Op, vs[1])
		//return fmt.Sprintf("%s(%s)", j.Op, strings.Join(vs, ","))
	}
}

func NewBinaryExpression(value1, value2 JavaValue, op string) *JavaExpression {
	return &JavaExpression{
		Values: []JavaValue{value1, value2},
		Op:     op,
	}
}

type FunctionCallExpression struct {
	JavaType     JavaType
	IsStatic     bool
	Object       JavaValue
	FunctionName string
	Arguments    []JavaValue
	FuncType     *JavaFuncType
}

func (f *FunctionCallExpression) Type() JavaType {
	return f.FuncType.ReturnType
}

func (f *FunctionCallExpression) String(funcCtx *FunctionContext) string {
	paramStrs := []string{}
	for _, arg := range f.Arguments {
		paramStrs = append(paramStrs, arg.String(funcCtx))
	}
	if f.IsStatic {
		return fmt.Sprintf("%s.%s(%s)", f.JavaType.String(funcCtx), f.FunctionName, strings.Join(paramStrs, ","))
	}
	return fmt.Sprintf("%s.%s(%s)", f.Object.String(funcCtx), f.FunctionName, strings.Join(paramStrs, ","))
}

func NewFunctionCallExpression(object JavaValue, name string, funcType *JavaFuncType) *FunctionCallExpression {
	return &FunctionCallExpression{
		FuncType:     funcType,
		Object:       object,
		FunctionName: name,
	}
}

type TernaryExpression struct {
	Condition  JavaValue
	TrueValue  JavaValue
	FalseValue JavaValue
}

func (t *TernaryExpression) Type() JavaType {
	return t.TrueValue.Type()
}

func (t *TernaryExpression) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("%s ? %s : %s", t.Condition.String(funcCtx), t.TrueValue.String(funcCtx), t.FalseValue.String(funcCtx))
}

func NewTernaryExpression(condition, trueValue, falseValue JavaValue) *TernaryExpression {
	return &TernaryExpression{
		Condition:  condition,
		TrueValue:  trueValue,
		FalseValue: falseValue,
	}
}
