package decompiler

import "fmt"

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
var _ JavaType = &JavaNull{}

type JavaArrayType struct {
	JavaType JavaType
	Length   int
}

func (j *JavaArrayType) String(funcCtx *FunctionContext) string  {
	return fmt.Sprintf("%s[]", j.JavaType.String(funcCtx))
}

func (j *JavaArrayType) IsJavaType() {

}

func NewJavaArrayType(typ JavaType, length int) *JavaArrayType {
	return &JavaArrayType{
		JavaType: typ,
		Length:   length,
	}
}

type JavaNull struct {
}

func (j JavaNull) String(funcCtx *FunctionContext) string  {
	return "null"
}

func (j JavaNull) IsJavaType() {
}

type JavaPrimer struct {
	Name string
}

func newJavaPrimer(name string) *JavaPrimer {
	return &JavaPrimer{
		Name: name,
	}
}

var (
	JavaString  = newJavaPrimer("String")
	JavaInteger = newJavaPrimer("Integer")
	JavaLong    = newJavaPrimer("Long")
	JavaDouble  = newJavaPrimer("Double")
	JavaFloat   = newJavaPrimer("Float")
)

func (j *JavaPrimer) String(funcCtx *FunctionContext) string  {
	return j.Name
}

func (j *JavaPrimer) IsJavaType() {}
