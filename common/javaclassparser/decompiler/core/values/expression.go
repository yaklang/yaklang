package values

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
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
	Typ    types.JavaType
}

func (j *JavaExpression) Type() types.JavaType {
	return j.Typ
}

func (j *JavaExpression) String(funcCtx *class_context.ClassContext) string {
	vs := []string{}
	for _, value := range j.Values {
		vs = append(vs, value.String(funcCtx))
	}
	if len(vs) == 1 {
		return fmt.Sprintf("%s(%s)", j.Op, vs[0])
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

func NewUnaryExpression(value1 JavaValue, op string, typ types.JavaType) *JavaExpression {
	return &JavaExpression{
		Values: []JavaValue{value1},
		Op:     op,
		Typ:    typ.Copy(),
	}
}
func NewBinaryExpression(value1, value2 JavaValue, op string, typ types.JavaType) *JavaExpression {
	return &JavaExpression{
		Values: []JavaValue{value1, value2},
		Op:     op,
		Typ:    typ.Copy(),
	}
}

type FunctionCallExpression struct {
	IsStatic     bool
	Object       JavaValue
	FunctionName string
	ClassName    string
	Arguments    []JavaValue
	FuncType     *types.JavaFuncType
}

func (f *FunctionCallExpression) Type() types.JavaType {
	return f.FuncType.ReturnType
}

func (f *FunctionCallExpression) IsSupperConstructorInvoke(funcCtx *class_context.ClassContext) bool {
	if f.FunctionName == "<init>" && f.ClassName == funcCtx.SupperClassName {
		return true
	}
	return false
}
func (f *FunctionCallExpression) String(funcCtx *class_context.ClassContext) string {
	paramStrs := []string{}
	for i, arg := range f.Arguments {
		argType := f.FuncType.ParamTypes[i]
		expectClassType, ok1 := argType.RawType().(*types.JavaClass)
		atcClassType, ok2 := arg.Type().RawType().(*types.JavaClass)
		if ok1 && ok2 && expectClassType.Name != atcClassType.Name {
			if expectClassType.Name != "java.lang.Object" {
				argStr := arg.String(funcCtx)
				argTypeStr := argType.String(funcCtx)
				arg = NewCustomValue(func(funcCtx *class_context.ClassContext) string {
					return fmt.Sprintf("(%s)(%s)", argTypeStr, argStr)
				}, func() types.JavaType {
					return argType
				})
			}
		}
		paramStrs = append(paramStrs, arg.String(funcCtx))
	}
	if f.FunctionName == "<init>" {
		if f.ClassName == funcCtx.ClassName {
			return fmt.Sprintf("%s(%s)", f.Object.String(funcCtx), strings.Join(paramStrs, ","))
		} else if f.ClassName == funcCtx.SupperClassName {
			return fmt.Sprintf("super(%s)", strings.Join(paramStrs, ","))
		}
	}

	if v, ok := f.Object.(*JavaClassValue); ok {
		if v.Type().RawType().(*types.JavaClass).Name == funcCtx.ClassName {
			return fmt.Sprintf("%s(%s)", f.FunctionName, strings.Join(paramStrs, ","))
		}
	}
	obj := UnpackSoltValue(f.Object)
	switch obj.(type) {
	case *JavaExpression, *TernaryExpression, *SlotValue:
		return fmt.Sprintf("(%s).%s(%s)", f.Object.String(funcCtx), f.FunctionName, strings.Join(paramStrs, ","))
	default:
		return fmt.Sprintf("%s.%s(%s)", f.Object.String(funcCtx), f.FunctionName, strings.Join(paramStrs, ","))
	}
}

func NewFunctionCallExpression(object JavaValue, methodMember *JavaClassMember, funcType *types.JavaFuncType) *FunctionCallExpression {
	return &FunctionCallExpression{
		FuncType:     funcType,
		Object:       object,
		FunctionName: methodMember.Member,
		ClassName:    methodMember.Name,
	}
}
