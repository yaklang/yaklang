package types

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"strings"
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
	JavaType JavaType
	Dim      int
}

func (j *JavaArrayType) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("%s%s", j.JavaType.String(funcCtx), strings.Repeat("[]", j.Dim))
}

func (j *JavaArrayType) IsJavaType() {

}

func NewJavaArrayType(typ JavaType) JavaType {
	if typ.IsArray() {
		return newJavaTypeWrap(&JavaArrayType{
			JavaType: typ.ElementType(),
			Dim:      typ.ArrayDim() + 1,
		})
	}
	return newJavaTypeWrap(&JavaArrayType{
		JavaType: typ,
		Dim:      1,
	})
}

type JavaClass struct {
	Name string
	JavaType
}

func (j *JavaClass) IsJavaType() {

}
func (j *JavaClass) Type() JavaType {
	return newJavaTypeWrap(j)
}

func (j *JavaClass) String(funcCtx *class_context.ClassContext) string {
	if funcCtx.ClassName == j.Name{

	}
	name := funcCtx.ShortTypeName(j.Name)
	return fmt.Sprintf("%s", name)
}
func NewJavaClass(typeName string) JavaType {
	return newJavaTypeWrap(&JavaClass{
		Name: typeName,
	})
}
