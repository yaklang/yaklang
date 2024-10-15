package values

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type CustomValue struct {
	StringFunc func(funcCtx *class_context.FunctionContext) string
	TypeFunc   func() types.JavaType
}

func (v *CustomValue) Type() types.JavaType {
	return v.TypeFunc()
}
func (v *CustomValue) String(funcCtx *class_context.FunctionContext) string {
	return v.StringFunc(funcCtx)
}
func NewCustomValue(stringFun func(funcCtx *class_context.FunctionContext) string, typeFunc func() types.JavaType) *CustomValue {
	return &CustomValue{
		StringFunc: stringFun,
		TypeFunc:   typeFunc,
	}
}
