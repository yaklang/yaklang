package statements

import (
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
)

type Statement interface {
	String(funcCtx *class_context.ClassContext) string
	ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId)
}

var _ Statement = &IfStatement{}
var _ Statement = &AssignStatement{}
var _ Statement = &CustomStatement{}
var _ Statement = &ForStatement{}
var _ Statement = &WhileStatement{}
var _ Statement = &DoWhileStatement{}
var _ Statement = &SwitchStatement{}
var _ Statement = &ReturnStatement{}
var _ Statement = &TryCatchStatement{}
var _ Statement = &ConditionStatement{}
var _ Statement = &GOTOStatement{}
var _ Statement = &NewStatement{}
var _ Statement = &ExpressionStatement{}
var _ Statement = &SynchronizedStatement{}
var _ Statement = &MiddleStatement{}
