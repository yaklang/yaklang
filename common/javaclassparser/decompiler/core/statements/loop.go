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
// normalizeDoWhileDecrementGuard detects the bytecode pattern where javac compiles a
// `while (i-- > 0) { body }` loop (or a for-loop with the decrement folded into the test) as a
// back-edge whose head is: `iinc i,-1; if (old_i > 0) body else break`. The structuring builds a
// do-while whose body is [i--; if (i > 0) { body } else { break }]. Because the standalone `i--`
// runs BEFORE the test, the test sees the *decremented* value and the loop body executes one
// fewer time than the original (n -> n-1). This is an off-by-one that corrupts algorithms whose
// iteration count matters (e.g. MD5-crypt B64.b64from24bit base64 packing).
//
// Fix: when the body is exactly [decrement-of-v, if(v cmp k, body, [break])], fold the decrement
// into the test as a POST-decrement so the test evaluates the pre-decrement value (matching the
// bytecode): `do { if ((v--) cmp k) { body } } while(true)`.
func NormalizeDoWhileDecrementGuard(body []Statement, funcCtx *class_context.ClassContext) []Statement {
	if len(body) < 2 {
		return body
	}
	// The leading decrement may appear as a bare JavaExpression or wrapped in an
	// ExpressionStatement, depending on the statement-list path that produced the body.
	var decExpr *values.JavaExpression
	switch v := body[0].(type) {
	case *ExpressionStatement:
		decExpr, _ = v.Expression.(*values.JavaExpression)
	case *values.JavaExpression:
		decExpr = v
	}
	if decExpr == nil {
		return body
	}
	if decExpr.Op != values.DEC || len(decExpr.Values) < 1 {
		return body
	}
	decRef, ok := values.UnpackSoltValue(decExpr.Values[0]).(*values.JavaRef)
	if !ok || decRef.VarUid == "" {
		return body
	}
	ifs, ok := body[1].(*IfStatement)
	if !ok || ifs.Condition == nil {
		return body
	}
	// Condition must be a binary comparison whose left operand is the SAME variable as the
	// decrement, so folding the decrement into it preserves semantics.
	cond, ok := ifs.Condition.(*values.JavaExpression)
	if !ok || len(cond.Values) != 2 {
		return body
	}
	condLeftRef, ok := values.UnpackSoltValue(cond.Values[0]).(*values.JavaRef)
	if !ok || condLeftRef.VarUid != decRef.VarUid {
		return body
	}
	// The if must be a loop guard: else-branch is a plain break.
	if !isPlainBreakList(ifs.ElseBody, funcCtx) {
		return body
	}
	// Build the post-decrement operand (renders `v--`) and splice it into the condition as the
	// left operand so the test reads `(v--) cmp k`, evaluating the pre-decrement value like the
	// bytecode.
	postDec := values.NewBinaryExpression(decRef, values.NewJavaLiteral(1, types.NewJavaPrimer(types.JavaInteger)), values.DEC, decRef.Type())
	newCond := values.NewBinaryExpression(postDec, cond.Values[1], cond.Op, cond.Typ)
	newIf := NewIfStatement(newCond, ifs.IfBody, ifs.ElseBody)
	out := make([]Statement, 0, len(body))
	out = append(out, newIf)
	out = append(out, body[2:]...)
	return out
}

func isPlainBreakList(sts []Statement, funcCtx *class_context.ClassContext) bool {
	if len(sts) != 1 {
		return false
	}
	return isPlainBreakStatement(sts[0], funcCtx)
}

func (w *DoWhileStatement) String(funcCtx *class_context.ClassContext) string {
	normalizedBody := NormalizeDoWhileDecrementGuard(w.Body, funcCtx)
	body := normalizeDoWhileBreakGuard(doWhileBodyString(normalizedBody, funcCtx))
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
