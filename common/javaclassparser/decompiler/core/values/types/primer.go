package types

import "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"

type JavaPrimer struct {
	Name string
}

func NewJavaPrimer(name string) JavaType {
	return newJavaTypeWrap(&JavaPrimer{
		Name: name,
	})
}

var (
	JavaChar    = "char"
	JavaInteger = "int"
	JavaLong    = "long"
	JavaDouble  = "double"
	JavaFloat   = "float"
	JavaBoolean = "boolean"
	JavaByte    = "byte"
	JavaShort   = "short"
	JavaString  = "String"
	JavaVoid    = "void"
)

func (j *JavaPrimer) String(funcCtx *class_context.ClassContext) string {
	return j.Name
}

func (j *JavaPrimer) IsJavaType() {}
