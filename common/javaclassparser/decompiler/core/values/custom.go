package values

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type CustomValue struct {
	Flag        string
	StringFunc  func(funcCtx *class_context.ClassContext) string
	TypeFunc    func() types.JavaType
	ReplaceFunc func(oldId *utils.VariableId, newId *utils.VariableId)
}

// ReplaceVar implements JavaValue.
func (v *CustomValue) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if v.ReplaceFunc != nil {
		v.ReplaceFunc(oldId, newId)
	}
}

func (v *CustomValue) Type() types.JavaType {
	return v.TypeFunc()
}
func (v *CustomValue) String(funcCtx *class_context.ClassContext) string {
	return v.StringFunc(funcCtx)
}
func NewCustomValue(stringFun func(funcCtx *class_context.ClassContext) string, typeFunc func() types.JavaType, replaceFunc ...func(oldId *utils.VariableId, newId *utils.VariableId)) *CustomValue {
	var rf func(oldId *utils.VariableId, newId *utils.VariableId)
	if len(replaceFunc) > 0 {
		rf = replaceFunc[0]
	}
	return &CustomValue{
		StringFunc:  stringFun,
		TypeFunc:    typeFunc,
		ReplaceFunc: rf,
	}
}
