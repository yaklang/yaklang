package values

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
	"strings"
)

type NewExpression struct {
	types.JavaType
	Length          []JavaValue
	ArgumentsGetter func() string
}

func NewNewArrayExpression(typ types.JavaType, length ...JavaValue) *NewExpression {
	return &NewExpression{
		JavaType: typ,
		Length:   length,
	}
}
func NewNewExpression(typ types.JavaType) *NewExpression {
	return &NewExpression{
		JavaType: typ,
	}
}
func (n *NewExpression) Type() types.JavaType {
	return n.JavaType
}

func (n *NewExpression) String(funcCtx *class_context.ClassContext) string {
	if n.IsArray() {
		s := fmt.Sprintf("new %s", n.ElementType().String(funcCtx))
		for _, l := range n.Length {
			s += fmt.Sprintf("[%v]", l.(JavaValue).String(funcCtx))
		}
		return s
	}
	var args string
	if n.ArgumentsGetter != nil {
		args = n.ArgumentsGetter()
	}
	return fmt.Sprintf("new %s(%s)", n.JavaType.String(funcCtx), args)
}

type JavaExpression struct {
	Values []JavaValue
	Op     string
}

func (j *JavaExpression) Type() types.JavaType {
	return j.Values[0].Type()
}

func (j *JavaExpression) String(funcCtx *class_context.ClassContext) string {
	vs := []string{}
	for _, value := range j.Values {
		vs = append(vs, value.String(funcCtx))
	}
	switch j.Op {
	case ADD:
		return fmt.Sprintf("(%s) + (%s)", vs[0], vs[1])
	case INC:
		return fmt.Sprintf("%s++", vs[0])
	case GT, SUB:
		return fmt.Sprintf("(%s) %s (%s)", vs[0], j.Op, vs[1])
	default:
		return fmt.Sprintf("(%s) %s (%s)", vs[0], j.Op, vs[1])
	}
}

func NewBinaryExpression(value1, value2 JavaValue, op string) *JavaExpression {
	return &JavaExpression{
		Values: []JavaValue{value1, value2},
		Op:     op,
	}
}

type FunctionCallExpression struct {
	IsStatic     bool
	Object       JavaValue
	FunctionName string
	Arguments    []JavaValue
	FuncType     *types.JavaFuncType
}

func (f *FunctionCallExpression) Type() types.JavaType {
	return f.FuncType.ReturnType
}

func (f *FunctionCallExpression) String(funcCtx *class_context.ClassContext) string {
	paramStrs := []string{}
	for _, arg := range f.Arguments {
		paramStrs = append(paramStrs, arg.String(funcCtx))
	}
	//if f.IsStatic {
	//	return fmt.Sprintf("%s.%s(%s)", f.JavaType.String(funcCtx), f.FunctionName, strings.Join(paramStrs, ","))
	//}
	//var objName string
	//if !reflect.ValueOf(f.Object).IsNil() {
	//	objName = f.Object.String(funcCtx)
	//}
	//if objName == "" {
	//	return fmt.Sprintf("%s(%s)", f.FunctionName, strings.Join(paramStrs, ","))
	//}
	if f.FunctionName == "<init>" {
		return fmt.Sprintf("%s(%s)", f.Object.String(funcCtx), strings.Join(paramStrs, ","))
	}
	if v, ok := f.Object.(*JavaClassValue); ok {
		if v.Type().RawType().(*types.JavaClass).Name == funcCtx.ClassName {
			return fmt.Sprintf("%s(%s)", f.FunctionName, strings.Join(paramStrs, ","))
		}
	}
	return fmt.Sprintf("%s.%s(%s)", f.Object.String(funcCtx), f.FunctionName, strings.Join(paramStrs, ","))
}

func NewFunctionCallExpression(object JavaValue, name string, funcType *types.JavaFuncType) *FunctionCallExpression {
	return &FunctionCallExpression{
		FuncType:     funcType,
		Object:       object,
		FunctionName: name,
	}
}
