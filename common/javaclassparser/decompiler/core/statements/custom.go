package statements

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
)

type CustomStatement struct {
	Name       string
	Info       any
	StringFunc func(funcCtx *class_context.ClassContext) string
	replaceVar func(oldId *utils.VariableId, newId *utils.VariableId)
}

// ReplaceVar implements Statement.
func (v *CustomStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	v.replaceVar(oldId, newId)
}

func (v *CustomStatement) String(funcCtx *class_context.ClassContext) string {
	return v.StringFunc(funcCtx)
}
func NewCustomStatement(stringFun func(funcCtx *class_context.ClassContext) string, replaceVar func(oldId *utils.VariableId, newId *utils.VariableId)) *CustomStatement {
	return &CustomStatement{
		StringFunc: stringFun,
		replaceVar: replaceVar,
	}
}
