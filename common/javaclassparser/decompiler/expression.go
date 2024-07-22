package decompiler

import (
	"fmt"
	"strings"
)

const (
	ADD = "add"
)

type JavaExpression struct {
	Values   []JavaValue
	Op       string
	JavaType JavaType
}

func (j *JavaExpression) Type() JavaType {
	return j.JavaType
}

func (j *JavaExpression) String() string {
	vs := []string{}
	for _, value := range j.Values {
		vs = append(vs, value.String())
	}
	return fmt.Sprintf("%s(%s)", j.Op, strings.Join(vs, ","))
}
func NewBinaryExpression(value1, value2 JavaValue, op string) *JavaExpression {
	return &JavaExpression{
		Values:   []JavaValue{value1, value2},
		Op:       op,
		JavaType: value2.Type(),
	}
}
