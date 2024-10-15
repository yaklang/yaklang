package types

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"strings"
)

var _ JavaType = &JavaClass{}
var _ JavaType = &JavaPrimer{}
var _ JavaType = &JavaArrayType{}
var _ JavaType = &javaNull{}
var _ JavaType = &JavaFuncType{}

type JavaFuncType struct {
	Desc       string
	ParamTypes []JavaType
	ReturnType JavaType
}

func (j JavaFuncType) String(funcCtx *class_context.FunctionContext) string {
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
	Length   []any
}

func (j *JavaArrayType) String(funcCtx *class_context.FunctionContext) string {
	return fmt.Sprintf("%s%s", j.JavaType.String(funcCtx), strings.Repeat("[]", len(j.Length)))
}

func (j *JavaArrayType) IsJavaType() {

}

func NewJavaArrayType(typ JavaType, lengths ...any) *JavaArrayType {
	return &JavaArrayType{
		JavaType: typ,
		Length:   lengths,
	}
}

type javaNull struct {
}

func (j javaNull) Type() JavaType {
	return j
}

func (j javaNull) String(funcCtx *class_context.FunctionContext) string {
	return "null"
}

func (j javaNull) IsJavaType() {
}

var JavaNull = javaNull{}

type JavaClass struct {
	Name string
	JavaType
}

func (j *JavaClass) IsJavaType() {

}
func (j *JavaClass) Type() JavaType {
	return j
}

func (j *JavaClass) String(funcCtx *class_context.FunctionContext) string {
	name := funcCtx.ShortTypeName(j.Name)
	return fmt.Sprintf("%s", name)
}
func NewJavaClass(typeName string) *JavaClass {
	return &JavaClass{
		Name: typeName,
	}
}
