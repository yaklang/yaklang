package core

import (
	"fmt"
	"strings"
)

const (
	Integer    = "integer"
	Long       = "long"
	Double     = "double"
	Float      = "float"
	NullObject = "null"
	Class      = "class"
	//MethodHandle,  // Only used for invokedynamic arguments
	MethodType = "method"
)

type JavaRawType interface {
	GetStackType() *StackType
}

type JavaType interface {
	String(funcCtx *FunctionContext) string
	IsJavaType()
}

var _ JavaType = &JavaClass{}
var _ JavaType = &JavaPrimer{}
var _ JavaType = &JavaArrayType{}
var _ JavaType = &javaNull{}
var _ JavaType = &JavaFuncType{}

type JavaFuncType struct {
	Desc       string
	Params     []JavaType
	ReturnType JavaType
}

func (j JavaFuncType) String(funcCtx *FunctionContext) string {
	return j.Desc
}

func (j JavaFuncType) IsJavaType() {

}

func NewJavaFuncType(desc string, params []JavaType, returnType JavaType) *JavaFuncType {
	return &JavaFuncType{
		Params:     params,
		ReturnType: returnType,
	}
}

type JavaArrayType struct {
	JavaType JavaType
	Length   []JavaValue // 支持多维数组
}

func (j *JavaArrayType) String(funcCtx *FunctionContext) string {
	return fmt.Sprintf("%s%s", j.JavaType.String(funcCtx), strings.Repeat("[]", len(j.Length)))
}

func (j *JavaArrayType) IsJavaType() {

}

func NewJavaArrayType(typ JavaType, length ...JavaValue) *JavaArrayType {
	return &JavaArrayType{
		JavaType: typ,
		Length:   length,
	}
}

type javaNull struct {
}

func (j javaNull) Type() JavaType {
	return j
}

func (j javaNull) String(funcCtx *FunctionContext) string {
	return "null"
}

func (j javaNull) IsJavaType() {
}

var JavaNull = javaNull{}

type JavaPrimer struct {
	Name string
}

func newJavaPrimer(name string) *JavaPrimer {
	return &JavaPrimer{
		Name: name,
	}
}

var (
	JavaChar    = newJavaPrimer("char")
	JavaInteger = newJavaPrimer("int")
	JavaLong    = newJavaPrimer("long")
	JavaDouble  = newJavaPrimer("double")
	JavaFloat   = newJavaPrimer("float")
	JavaBoolean = newJavaPrimer("boolean")
	JavaByte    = newJavaPrimer("byte")
	JavaShort   = newJavaPrimer("short")

	JavaString = newJavaPrimer("String")
	JavaVoid   = newJavaPrimer("void")
)

func (j *JavaPrimer) String(funcCtx *FunctionContext) string {
	return j.Name
}

func (j *JavaPrimer) IsJavaType() {}
