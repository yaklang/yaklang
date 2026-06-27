package statements

import (
	"fmt"
	"strings"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/utils"

	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/class_context"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values"
	"github.com/yaklang/yaklang/common/javaclassparser/decompiler/core/values/types"
)

type ConditionStatement struct {
	Condition values.JavaValue
	Neg       bool
	Callback  func(values.JavaValue)
	// TernaryChainArm mirrors OpCode.TernaryChainArm: this condition supplies a DISTINCT nested
	// ternary arm and therefore must not be folded into a short-circuit &&/|| by MergeIf.
	TernaryChainArm bool
}

// ReplaceVar implements Statement.
func (r *ConditionStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	r.Condition.ReplaceVar(oldId, newId)
}

func (r *ConditionStatement) String(funcCtx *class_context.ClassContext) string {
	// A ConditionStatement is an intermediate structuring placeholder that should be consumed by
	// IfRewriter before reaching output. If one leaks (a structuring gap at a complex merge point),
	// rendering it as bare `if cond` (no brackets, no body) produces INVALID Java that fails syntax
	// validation and stubs the ENTIRE method. As a last-resort safety net, render it as a valid
	// `if (cond){}` — syntactically parseable Java with an empty body — so the method degrades
	// gracefully (one branch is empty) instead of being fully stubbed.
	return fmt.Sprintf("if (%s){}", r.Condition.String(funcCtx))
}

// isBoolPrimer reports whether v carries a non-nil boolean primitive type. It guards against the
// nil Type() that incomplete stack simulation can produce, so the boolean-comparison folding below
// never nil-dereferences (which would panic the whole method into a stub).
func isBoolPrimer(v values.JavaValue) bool {
	if v == nil {
		return false
	}
	t := v.Type()
	if t == nil {
		return false
	}
	p, ok := t.RawType().(*types.JavaPrimer)
	return ok && p.Name == types.JavaBoolean
}

func NewConditionStatement(cmp values.JavaValue, op string) *ConditionStatement {
	if t := cmp.Type(); t != nil {
		t.ResetType(types.NewJavaPrimer(types.JavaBoolean))
	}
	if v, ok := cmp.(*values.JavaCompare); ok {
		if op == values.NEQ {
			if literal, ok := v.JavaValue2.(*values.JavaLiteral); ok {
				if isBoolPrimer(v.JavaValue1) {
					if literal.Data == 0 {
						return &ConditionStatement{
							Condition: v.JavaValue1,
						}
					}
					if literal.Data == 1 {
						return &ConditionStatement{
							Condition: values.NewUnaryExpression(v.JavaValue1, values.Not, types.NewJavaPrimer(types.JavaBoolean)),
						}
					}
				}
			}
		}
		if op == values.EQ {
			if literal, ok := v.JavaValue2.(*values.JavaLiteral); ok {
				if isBoolPrimer(v.JavaValue1) {
					if literal.Data == 0 {
						return &ConditionStatement{
							Condition: values.NewUnaryExpression(v.JavaValue1, values.Not, types.NewJavaPrimer(types.JavaBoolean)),
						}
					}
					if literal.Data == 1 {
						return &ConditionStatement{
							Condition: v.JavaValue1,
						}
					}
				}
			}
		}
		return &ConditionStatement{
			Condition: values.NewBinaryExpression(v.JavaValue1, v.JavaValue2, op, types.NewJavaPrimer(types.JavaBoolean)),
		}
	} else {
		return &ConditionStatement{
			Condition: cmp,
		}
	}
}

type ReturnStatement struct {
	JavaValue values.JavaValue
}

// ReplaceVar implements Statement.
func (r *ReturnStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if r.JavaValue != nil {
		r.JavaValue.ReplaceVar(oldId, newId)
	}
}

func (r *ReturnStatement) String(funcCtx *class_context.ClassContext) string {
	if r.JavaValue == nil {
		return "return"
	}
	expr := r.JavaValue.String(funcCtx)
	// Narrowing cast for char/byte/short return types: bytecode stores char/byte/short
	// literals as ints (bipush/sipush/iconst), so a method returning char whose body
	// returns `cond ? 102 : 101` renders int literals that javac rejects ("possible
	// lossy conversion from int to char"). When the declared return type is narrower than
	// int and the returned value is int-typed, wrap it in an explicit cast. This is a
	// pure rendering fix — the recompiled bytecode is behaviorally identical.
	if cast := narrowingReturnCast(funcCtx, r.JavaValue); cast != "" {
		return fmt.Sprintf("return (%s) (%s)", cast, expr)
	}
	return fmt.Sprintf("return %s", expr)
}

// narrowingReturnCast returns the cast type name ("char"/"byte"/"short") when the enclosing
// method's declared return type is a narrowing-of-int and the returned value is int-typed,
// otherwise "". The cast lets the emitted source recompile without "possible lossy
// conversion from int to char/byte/short" errors.
func narrowingReturnCast(funcCtx *class_context.ClassContext, v values.JavaValue) string {
	ft, ok := funcCtx.FunctionType.(*types.JavaFuncType)
	if !ok || ft == nil || ft.ReturnType == nil {
		return ""
	}
	retStr := ft.ReturnType.String(&class_context.ClassContext{})
	valStr := v.Type().RawType().String(&class_context.ClassContext{})
	if valStr != "int" {
		return ""
	}
	switch retStr {
	case "char", "byte", "short":
		return retStr
	}
	return ""
}

func NewReturnStatement(value values.JavaValue) *ReturnStatement {
	return &ReturnStatement{
		JavaValue: value,
	}
}

type StackAssignStatement struct {
	Id        int
	JavaValue *values.JavaRef
}

// ReplaceVar implements Statement.
func (a *StackAssignStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.JavaValue.ReplaceVar(oldId, newId)
}

func (a *StackAssignStatement) String(funcCtx *class_context.ClassContext) string {
	return a.JavaValue.String(funcCtx)
}
func NewStackAssignStatement(id int, value *values.JavaRef) *StackAssignStatement {
	return &StackAssignStatement{
		Id:        id,
		JavaValue: value,
	}
}

type AssignStatement struct {
	LeftValue   values.JavaValue
	ArrayMember *values.JavaArrayMember
	JavaValue   values.JavaValue
	IsDeclare   bool
	IsFirst     bool
}

// ReplaceVar implements Statement.
func (a *AssignStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	if a.LeftValue != nil {
		a.LeftValue.ReplaceVar(oldId, newId)
	}
	if a.ArrayMember != nil {
		a.ArrayMember.ReplaceVar(oldId, newId)
	}
	if a.JavaValue != nil {
		a.JavaValue.ReplaceVar(oldId, newId)
	}
}

func (a *AssignStatement) String(funcCtx *class_context.ClassContext) string {
	if a.IsDeclare {
		if a.LeftValue == nil {
			return values.EmptySlotValuePlaceholder
		}
		return fmt.Sprintf("%s %s", a.LeftValue.Type().String(funcCtx), a.LeftValue.String(funcCtx))
	}
	if a.ArrayMember != nil {
		if a.JavaValue == nil {
			return fmt.Sprintf("%s = %s", a.ArrayMember.String(funcCtx), values.EmptySlotValuePlaceholder)
		}
		return fmt.Sprintf("%s = %s", a.ArrayMember.String(funcCtx), a.JavaValue.String(funcCtx))
	}
	if a.LeftValue == nil || a.JavaValue == nil {
		left := values.EmptySlotValuePlaceholder
		right := values.EmptySlotValuePlaceholder
		if a.LeftValue != nil {
			left = a.LeftValue.String(funcCtx)
		}
		if a.JavaValue != nil {
			right = a.JavaValue.String(funcCtx)
		}
		return fmt.Sprintf("%s = %s", left, right)
	}
	assign := fmt.Sprintf("%s = %s", a.LeftValue.String(funcCtx), a.JavaValue.String(funcCtx))
	if a.IsFirst {
		// For `T x = null`, the initializer's static type is java.lang.Object, but the
		// variable's declared type is its (possibly refined) ref type — using the initializer
		// type would emit `Object x = null` even after the slot adopted a concrete type, and
		// `return x` would then mismatch the method's return type. Prefer the variable type
		// for a null initializer; for every other case this is identical to the value type.
		declType := a.JavaValue.Type()
		if lit, ok := a.JavaValue.(*values.JavaLiteral); ok && fmt.Sprint(lit.Data) == "null" {
			declType = a.LeftValue.Type()
		}
		// Either side's type can be nil under incomplete simulation; fall back to the other
		// side rather than dereferencing nil (which panicked the whole method into a stub).
		if declType == nil {
			declType = a.LeftValue.Type()
		}
		if declType == nil {
			declType = a.JavaValue.Type()
		}
		if declType == nil {
			// No recoverable declared type. Emit the placeholder so the dumper's safety net
			// degrades this method cleanly instead of crashing.
			return values.EmptySlotValuePlaceholder + " " + assign
		}
		if _, ok := declType.RawType().(*types.JavaMultiCatchType); ok {
			// A multi-catch union type is legal only inside `catch (A | B e)`. If the exception
			// value is hoisted into an ordinary local (`cause = e` after the catch), render it as
			// a common Throwable subtype so the declaration remains valid Java.
			declType = types.NewJavaClass("java.lang.Exception")
		}
		return declType.String(funcCtx) + " " + assign
	} else {
		return assign
	}
}

type ForStatement struct {
	InitVar       Statement
	Condition     *ConditionStatement
	EndExp        Statement
	SubStatements []Statement
}

// ReplaceVar implements Statement.
func (f *ForStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	f.InitVar.ReplaceVar(oldId, newId)
	f.Condition.ReplaceVar(oldId, newId)
	f.EndExp.ReplaceVar(oldId, newId)
	for _, st := range f.SubStatements {
		st.ReplaceVar(oldId, newId)
	}
}

func NewForStatement(subStatements []Statement) *ForStatement {
	return &ForStatement{
		InitVar:       subStatements[0],
		Condition:     subStatements[1].(*ConditionStatement),
		EndExp:        subStatements[len(subStatements)-2],
		SubStatements: subStatements[2 : len(subStatements)-2],
	}
}
func (f *ForStatement) String(funcCtx *class_context.ClassContext) string {
	datas := []string{}
	datas = append(datas, f.InitVar.String(funcCtx))
	datas = append(datas, f.Condition.String(funcCtx))
	datas = append(datas, f.EndExp.String(funcCtx))
	statementStr := []string{}
	for _, statement := range f.SubStatements {
		statementStr = append(statementStr, statement.String(funcCtx))
	}
	s := fmt.Sprintf("for(%s; %s; %s) {\n%s\n}", datas[0], datas[1], datas[2], strings.Join(statementStr, "\n"))
	return s
}

func NewArrayMemberAssignStatement(m *values.JavaArrayMember, value values.JavaValue) *AssignStatement {
	return &AssignStatement{
		ArrayMember: m,
		JavaValue:   value,
	}
}

func NewDeclareStatement(leftVal values.JavaValue) *AssignStatement {
	return &AssignStatement{
		LeftValue: leftVal,
		IsDeclare: true,
	}
}
func NewAssignStatement(leftVal, value values.JavaValue, isFirst bool) *AssignStatement {
	if value == nil || leftVal == nil || value.Type() == nil || leftVal.Type() == nil {
		// Guard against nil values/types in malformed bytecode: rather than panicking
		// (which forces the whole method into a stub), create the assignment as-is.
		// The type merge is skipped when either side has no type.
	}

	if value.Type() != nil && leftVal.Type() != nil {
		value.Type().ResetType(leftVal.Type())
	}
	return &AssignStatement{
		LeftValue: leftVal,
		JavaValue: value,
		IsFirst:   isFirst,
	}
}

type IfStatement struct {
	Condition values.JavaValue
	IfBody    []Statement
	ElseBody  []Statement
}

func (g *IfStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	g.Condition.ReplaceVar(oldId, newId)
	for _, st := range g.IfBody {
		st.ReplaceVar(oldId, newId)
	}
	for _, st := range g.ElseBody {
		st.ReplaceVar(oldId, newId)
	}
}

func (g *IfStatement) String(funcCtx *class_context.ClassContext) string {
	getBody := func(sts []Statement) string {
		var res []string
		for _, st := range sts {
			res = append(res, st.String(funcCtx))
		}
		return strings.Join(res, "\n")
	}
	return fmt.Sprintf("if (%s){\n"+
		"%s\n"+
		"}else{\n"+
		"%s\n"+
		"}", g.Condition.String(funcCtx), getBody(g.IfBody), getBody(g.ElseBody))
}
func NewIfStatement(condition values.JavaValue, ifBody, elseBody []Statement) *IfStatement {
	return &IfStatement{
		Condition: condition,
		IfBody:    ifBody,
		ElseBody:  elseBody,
	}
}

type GOTOStatement struct {
	ToStatement int
}

// ReplaceVar implements Statement.
func (g *GOTOStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (g *GOTOStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("goto: %d", g.ToStatement)
}
func NewGOTOStatement() *GOTOStatement {
	return &GOTOStatement{}
}

type NewStatement struct {
	Class *types.JavaClass
}

// ReplaceVar implements Statement.
func (a *NewStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.Class.ReplaceVar(oldId, newId)
}

func (a *NewStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("new %s()", a.Class.Name)
}

func NewNewStatement(class *types.JavaClass) *NewStatement {
	return &NewStatement{
		Class: class,
	}
}

type ExpressionStatement struct {
	Expression values.JavaValue
}

// ReplaceVar implements Statement.
func (a *ExpressionStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.Expression.ReplaceVar(oldId, newId)
}

func (a *ExpressionStatement) String(funcCtx *class_context.ClassContext) string {
	return a.Expression.String(funcCtx)
}

func NewExpressionStatement(v values.JavaValue) *ExpressionStatement {
	return &ExpressionStatement{
		Expression: v,
	}
}

type CaseItem struct {
	IsDefault bool
	IntValue  int
	Body      []Statement
}

func NewCaseItem(v int, body []Statement) *CaseItem {
	return &CaseItem{
		Body:     body,
		IntValue: v,
	}
}

type SwitchStatement struct {
	Value values.JavaValue
	Cases []*CaseItem
}

// ReplaceVar implements Statement.
func (a *SwitchStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	a.Value.ReplaceVar(oldId, newId)
	for _, c := range a.Cases {
		for _, st := range c.Body {
			st.ReplaceVar(oldId, newId)
		}
	}
}

func (a *SwitchStatement) String(funcCtx *class_context.ClassContext) string {
	casesStrs := []string{}
	for _, c := range a.Cases {
		if c.IsDefault {
			casesStrs = append(casesStrs, fmt.Sprintf("default:\n%s", StatementsString(c.Body, funcCtx)))
			continue
		}
		casesStrs = append(casesStrs, fmt.Sprintf("case %d:\n%s", c.IntValue, StatementsString(c.Body, funcCtx)))
	}
	return fmt.Sprintf("switch(%s) {\n%s\n}", a.Value.String(funcCtx), strings.Join(casesStrs, "\n"))
}

func NewSwitchStatement(value values.JavaValue, cases []*CaseItem) *SwitchStatement {
	return &SwitchStatement{
		Value: value,
		Cases: cases,
	}
}

const (
	MiddleSwitch   = "switch"
	MiddleTryStart = "tryStart"
)

type MiddleStatement struct {
	Data any
	Flag string
}

// ReplaceVar implements Statement.
func (a *MiddleStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
}

func (a *MiddleStatement) String(funcCtx *class_context.ClassContext) string {
	return a.Flag
}

func NewMiddleStatement(flag string, d any) *MiddleStatement {
	return &MiddleStatement{
		Flag: flag,
		Data: d,
	}
}

type SynchronizedStatement struct {
	Argument values.JavaValue
	Body     []Statement
}

// ReplaceVar implements Statement.
func (s *SynchronizedStatement) ReplaceVar(oldId *utils.VariableId, newId *utils.VariableId) {
	s.Argument.ReplaceVar(oldId, newId)
	for _, st := range s.Body {
		st.ReplaceVar(oldId, newId)
	}
}

func NewSynchronizedStatement(val values.JavaValue, body []Statement) *SynchronizedStatement {
	return &SynchronizedStatement{Argument: val, Body: body}
}

func (s *SynchronizedStatement) String(funcCtx *class_context.ClassContext) string {
	return fmt.Sprintf("synchronized(%s) {\n%s\n}", s.Argument.String(funcCtx), StatementsString(s.Body, funcCtx))
}
