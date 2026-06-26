package statements

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
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
	body := normalizeDoWhileBreakGuard(doWhileBodyString(w.Body, funcCtx))
	s := fmt.Sprintf("do{\n%s\n}while(%s)", body, w.ConditionValue.String(funcCtx))
	if w.Label != "" {
		return fmt.Sprintf("%s: %s", w.Label, s)
	}
	return s
}

func doWhileBodyString(body []Statement, funcCtx *class_context.ClassContext) string {
	res := make([]string, 0, len(body))
	for _, st := range body {
		if ifs, ok := st.(*IfStatement); ok && len(ifs.IfBody) == 1 && len(ifs.ElseBody) > 0 && isPlainBreakStatement(ifs.IfBody[0], funcCtx) && ifs.Condition != nil {
			conditionText := strings.TrimSpace(ifs.Condition.String(funcCtx))
			if shouldInvertDoWhileBreakGuard(conditionText) {
				condition := values.SimplifyConditionValue(values.NewUnaryExpression(
					ifs.Condition,
					values.Not,
					types.NewJavaPrimer(types.JavaBoolean),
				))
				res = append(res, fmt.Sprintf("if (%s){\n%s\n}else{\n%s\n}", condition.String(funcCtx), StatementsString(ifs.IfBody, funcCtx), StatementsString(ifs.ElseBody, funcCtx)))
				continue
			}
		}
		res = append(res, st.String(funcCtx))
	}
	return strings.Join(res, "\n")
}

func isPlainBreakStatement(st Statement, funcCtx *class_context.ClassContext) bool {
	_, ok := st.(*CustomStatement)
	return ok && strings.TrimSpace(st.String(funcCtx)) == "break"
}

func normalizeDoWhileBreakGuard(body string) string {
	const prefix = "if ("
	const marker = "){\nbreak\n}else{"
	if !strings.HasPrefix(body, prefix) {
		return body
	}
	idx := strings.Index(body, marker)
	if idx <= len(prefix) {
		return body
	}
	condition := body[len(prefix):idx]
	if !shouldInvertDoWhileBreakGuard(condition) {
		return body
	}
	return prefix + "!(" + condition + ")" + body[idx:]
}

func shouldInvertDoWhileBreakGuard(condition string) bool {
	condition = strings.TrimSpace(condition)
	if condition == "" || strings.HasPrefix(condition, "!") {
		return false
	}
	if strings.Contains(condition, ">=") || strings.Contains(condition, ">") ||
		strings.Contains(condition, "==") || strings.Contains(condition, "!=") {
		return false
	}
	return strings.Contains(condition, "<")
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
