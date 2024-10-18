package types

import "github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"

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

type JavaType interface {
	String(funcCtx *class_context.ClassContext) string
	IsJavaType()
}

var _ JavaType = &JavaClass{}
var _ JavaType = &JavaPrimer{}
var _ JavaType = &JavaArrayType{}
var _ JavaType = &javaNull{}
var _ JavaType = &JavaFuncType{}
