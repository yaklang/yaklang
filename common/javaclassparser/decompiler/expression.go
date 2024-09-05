package decompiler

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
	Length  int
	JavaType
}

func NewNewArrayExpression(typ JavaType, length int) *NewExpression {
	return &NewExpression{
		JavaType: typ,
		IsArray:  true,
		Length:   length,
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
		typ := n.JavaType.(*JavaArrayType).JavaType
		return fmt.Sprintf("new %s[%d]", typ.String(funcCtx), n.Length)
	}
	return fmt.Sprintf("new %s", n.JavaType.String(funcCtx))
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
		return fmt.Sprintf("%s + %s", vs[0], vs[1])
	case INC:
		return fmt.Sprintf("%s += %s", vs[0], vs[1])

	default:
		return fmt.Sprintf("%s(%s)", j.Op, strings.Join(vs, ","))
	}
}

func NewBinaryExpression(value1, value2 JavaValue, op string) *JavaExpression {
	return &JavaExpression{
		Values: []JavaValue{value1, value2},
		Op:     op,
	}
}
