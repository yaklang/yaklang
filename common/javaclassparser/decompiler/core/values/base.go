package values

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type JavaValue interface {
	String(funcCtx *class_context.ClassContext) string
	Type() types.JavaType
}

var (
	_ JavaValue = &JavaRef{}
	_ JavaValue = &JavaArray{}
	_ JavaValue = &JavaLiteral{}
	_ JavaValue = &types.JavaClass{}
	_ JavaValue = &JavaClassMember{}
	_ JavaValue = &JavaExpression{}
	_ JavaValue = &NewExpression{}
	_ JavaValue = &FunctionCallExpression{}
	_ JavaValue = &RefMember{}
	_ JavaValue = &JavaCompare{}
	_ JavaValue = &JavaClassValue{}
	//_ JavaValue = types.JavaNull
)
