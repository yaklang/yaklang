package types

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
)

type JavaFuncType struct {
	Desc       string
	ParamTypes []JavaType
	ReturnType JavaType
}

func (j JavaFuncType) String(funcCtx *class_context.ClassContext) string {
	return j.Desc
}

func (j JavaFuncType) IsJavaType() {

}

func NewJavaFuncType(desc string, params []JavaType, returnType JavaType) *JavaFuncType {
	return &JavaFuncType{
		ParamTypes: params,
		ReturnType: returnType,
	}
}

type JavaArrayType struct {
	JavaType  JavaType
	Dimension int
}

func (j *JavaArrayType) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("%s%s", j.JavaType.String(funcCtx), strings.Repeat("[]", j.Dimension))
}

func (j *JavaArrayType) IsJavaType() {

}

func NewJavaArrayType(typ JavaType) JavaType {
	if typ.IsArray() {
		return newJavaTypeWrap(&JavaArrayType{
			JavaType:  typ.ElementType(),
			Dimension: typ.ArrayDim() + 1,
		})
	}
	return newJavaTypeWrap(&JavaArrayType{
		JavaType:  typ,
		Dimension: 1,
	})
}

type JavaClass struct {
	Name string
	JavaType
}

// ReplaceVar implements values.JavaValue.
func (j *JavaClass) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (j *JavaClass) IsJavaType() {

}
func (j *JavaClass) Type() JavaType {
	return newJavaTypeWrap(j)
}

func (j *JavaClass) String(funcCtx *class_context.ClassContext) string {
	name := funcCtx.ShortTypeName(j.Name)
	return fmt.Sprintf("%s", name)
}
func NewJavaClass(typeName string) JavaType {
	if strings.HasPrefix(typeName, "[") {
		t, err := ParseDescriptor(typeName)
		if err != nil {
			panic("parse type failed")
		}
		return t
	}
	return newJavaTypeWrap(&JavaClass{
		Name: typeName,
	})
}
