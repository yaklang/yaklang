package values

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type JavaCompare struct {
	JavaValue1, JavaValue2 JavaValue
}

func (j *JavaCompare) Type() types.JavaType {
	return types.JavaBoolean
}

func (j *JavaCompare) String(funcCtx *class_context.FunctionContext) string {
	return fmt.Sprintf("%s compare %s", j.JavaValue1.String(funcCtx), j.JavaValue2.String(funcCtx))
}

func NewJavaCompare(v1, v2 JavaValue) *JavaCompare {
	return &JavaCompare{
		JavaValue1: v1,
		JavaValue2: v2,
	}
}

type LambdaFuncRef struct {
	Id           int
	JavaType     types.JavaType
	LambdaRender func(funcCtx *class_context.FunctionContext) string
}

func (j *LambdaFuncRef) Type() types.JavaType {
	return j.JavaType
}

func (j *LambdaFuncRef) String(funcCtx *class_context.FunctionContext) string {
	if j.LambdaRender != nil {
		return j.LambdaRender(funcCtx)
	}
	return fmt.Sprintf("getLambda(%d)", j.Id)
}

func NewLambdaFuncRef(id int, typ types.JavaType) *LambdaFuncRef {
	return &LambdaFuncRef{
		Id:       id,
		JavaType: typ,
	}
}
