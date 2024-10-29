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
	javaType
	ResetType(t JavaType)
	IsArray() bool
	ElementType() JavaType
	ArrayDim() int
	FunctionType() *JavaFuncType
	RawType() javaType
	Copy() JavaType
}

type JavaTypeWrap struct {
	javaType
}

func (j *JavaTypeWrap) Copy() JavaType {
	return &JavaTypeWrap{
		javaType: j.javaType,
	}
}
func (j *JavaTypeWrap) ArrayDim() int {
	v, ok := j.javaType.(*JavaArrayType)
	if ok {
		return v.Dimension
	}
	return 0
}
func (j *JavaTypeWrap) RawType() javaType {
	return j.javaType
}
func (j *JavaTypeWrap) FunctionType() *JavaFuncType {
	v, ok := j.javaType.(*JavaFuncType)
	if ok {
		return v
	}
	return nil
}
func (j *JavaTypeWrap) IsArray() bool {
	_, ok := j.javaType.(*JavaArrayType)
	return ok
}
func (j *JavaTypeWrap) ElementType() JavaType {
	v, ok := j.javaType.(*JavaArrayType)
	if ok {
		if v.Dimension == 1 {
			return v.JavaType
		} else {
			return newJavaTypeWrap(&JavaArrayType{
				JavaType:  v.JavaType,
				Dimension: v.Dimension - 1,
			})
		}
	}
	return nil
}
func (j *JavaTypeWrap) ResetType(t JavaType) {
	if t.String(&class_context.ClassContext{}) == JavaBoolean {
		j.javaType = t.RawType()
	}
	if j.String(&class_context.ClassContext{}) == JavaVoid {
		j.javaType = t.RawType()
	}
}
func newJavaTypeWrap(t javaType) *JavaTypeWrap {
	return &JavaTypeWrap{
		javaType: t,
	}
}

type javaType interface {
	String(funcCtx *class_context.ClassContext) string
	IsJavaType()
}

var _ javaType = &JavaClass{}
var _ javaType = &JavaPrimer{}
var _ javaType = &JavaArrayType{}
var _ javaType = &JavaFuncType{}
