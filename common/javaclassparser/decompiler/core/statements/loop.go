package statements

import (
	"fmt"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
)

type DoWhileStatement struct {
	Label          string
	ConditionValue values.JavaValue
	Body           []Statement
}

// ReplaceVar implements Statement.
func (w *DoWhileStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	w.ConditionValue.ReplaceVar(oldId, newId)
	for _, st := range w.Body {
		st.ReplaceVar(oldId, newId)
	}
}

func NewDoWhileStatement(condition values.JavaValue, body []Statement) *DoWhileStatement {
	return &DoWhileStatement{
		ConditionValue: condition,
		Body:           body,
	}
}
func (w *DoWhileStatement) String(funcCtx *class_context.ClassContext) string {
	s := fmt.Sprintf("do{\n%s\n}while(%s)", StatementsString(w.Body, funcCtx), w.ConditionValue.String(funcCtx))
	if w.Label != "" {
		return fmt.Sprintf("%s: %s", w.Label, s)
	}
	return s
}

type WhileStatement struct {
	ConditionValue values.JavaValue
	Body           []Statement
}

// ReplaceVar implements Statement.
func (w *WhileStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	w.ConditionValue.ReplaceVar(oldId, newId)
	for _, st := range w.Body {
		st.ReplaceVar(oldId, newId)
	}
}

func NewWhileStatement(condition values.JavaValue, body []Statement) *WhileStatement {
	return &WhileStatement{
		ConditionValue: condition,
		Body:           body,
	}
}
func (w *WhileStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("while(%s) {\n%s\n}", w.ConditionValue.String(funcCtx), StatementsString(w.Body, funcCtx))
}

type TryCatchStatement struct {
	Exception   []*values.JavaRef
	TryBody     []Statement
	CatchBodies [][]Statement
}

// ReplaceVar implements Statement.
func (w *TryCatchStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	for _, exception := range w.Exception {
		exception.ReplaceVar(oldId, newId)
	}
	for _, body := range w.TryBody {
		body.ReplaceVar(oldId, newId)
	}

}

func NewTryCatchStatement(body1 []Statement, body2 [][]Statement) *TryCatchStatement {
	return &TryCatchStatement{
		TryBody:     body1,
		CatchBodies: body2,
	}
}
func (w *TryCatchStatement) String(funcCtx *class_context.ClassContext) string {
	bodies := []string{}
	for _, body := range w.CatchBodies {
		bodies = append(bodies, StatementsString(body, funcCtx))
	}
	s := fmt.Sprintf("try{\n%s\n}", StatementsString(w.TryBody, funcCtx))
	for _, body := range bodies {
		s += fmt.Sprintf("catch{\n%s\n}", body)
	}
	return s
}
