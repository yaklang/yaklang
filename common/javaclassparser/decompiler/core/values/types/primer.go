package types

import "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"

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
	JavaString  = newJavaPrimer("String")
	JavaVoid    = newJavaPrimer("void")
)

func (j *JavaPrimer) String(funcCtx *class_context.FunctionContext) string {
	return j.Name
}

func (j *JavaPrimer) IsJavaType() {}
