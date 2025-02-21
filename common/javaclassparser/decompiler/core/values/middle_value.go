package values

import (
	"fmt"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type JavaCompare struct {
	JavaValue1, JavaValue2 JavaValue
}

// ReplaceVar implements JavaValue.
func (j *JavaCompare) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	j.JavaValue1.ReplaceVar(oldId, newId)
	j.JavaValue2.ReplaceVar(oldId, newId)
}

func (j *JavaCompare) Type() types.JavaType {
	return types.NewJavaPrimer(types.JavaBoolean)
}

func (j *JavaCompare) String(funcCtx *class_context.ClassContext) string {
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
	LambdaRender func(funcCtx *class_context.ClassContext) string
	Arguments    []JavaValue
}

func (j *LambdaFuncRef) Type() types.JavaType {
	return j.JavaType
}

func (j *LambdaFuncRef) String(funcCtx *class_context.ClassContext) string {
	if j.LambdaRender != nil {
		return j.LambdaRender(funcCtx)
	}
	args := ""
	for _, arg := range j.Arguments {
		args += arg.String(funcCtx) + ","
	}
	return fmt.Sprintf("getLambda(%d)(%s)", j.Id, args)
}
