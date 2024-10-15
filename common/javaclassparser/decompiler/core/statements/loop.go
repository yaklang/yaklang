package statements

import (
	"fmt"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

type DoWhileStatement struct {
	ConditionValue values.JavaValue
	Body           []Statement
}

func NewDoWhileStatement(condition values.JavaValue, body []Statement) *DoWhileStatement {
	return &DoWhileStatement{
		ConditionValue: condition,
		Body:           body,
	}
}
func (w *DoWhileStatement) String(funcCtx *class_context.FunctionContext) string {
	return fmt.Sprintf("do{\n%s\n}while(%s)", StatementsString(w.Body, funcCtx), w.ConditionValue.String(funcCtx))
}

type WhileStatement struct {
	ConditionValue values.JavaValue
	Body           []Statement
}

func NewWhileStatement(condition values.JavaValue, body []Statement) *WhileStatement {
	return &WhileStatement{
		ConditionValue: condition,
		Body:           body,
	}
}
func (w *WhileStatement) String(funcCtx *class_context.FunctionContext) string {
	return fmt.Sprintf("while(%s) {\n%s\n}", w.ConditionValue.String(funcCtx), StatementsString(w.Body, funcCtx))
}
