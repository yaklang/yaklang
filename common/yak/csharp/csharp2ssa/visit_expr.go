package csharp2ssa

import (
	"github.com/yaklang/antlr/v4"
	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

// 表达式编译入口：
//
//	expression: non_assignment_expression (assignment_operator expression)?
//
// 二元/一元运算按文法优先级逐层下沉，最终落到 primary_expression（见 visit_primary.go）。
// 左值（赋值目标、++/--、out/ref 实参）统一走 unwrapToPrimary + VisitPrimaryLeftValue。

func (b *singleFileBuilder) VisitExpression(raw csharpparser.IExpressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.ExpressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Assignment_operator() == nil || i.Expression() == nil {
		return b.VisitNonAssignmentExpression(i.Non_assignment_expression())
	}
	return b.visitAssignment(i.Non_assignment_expression(), i.Assignment_operator(), i.Expression())
}

func (b *singleFileBuilder) VisitNonAssignmentExpression(raw csharpparser.INon_assignment_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Non_assignment_expressionContext)
	if !ok || i == nil {
		return nil
	}
	switch {
	case i.Conditional_expression() != nil:
		return b.VisitConditionalExpression(i.Conditional_expression())
	case i.Lambda_expression() != nil:
		return b.VisitLambdaExpression(i.Lambda_expression())
	case i.Query_expression() != nil:
		return b.VisitQueryExpression(i.Query_expression())
	case i.Declaration_expression() != nil:
		return b.VisitDeclarationExpression(i.Declaration_expression())
	}
	return b.EmitUndefined(i.GetText())
}

// VisitDeclarationExpression handles `out var x` / `T x` appearing in expression position.
func (b *singleFileBuilder) VisitDeclarationExpression(raw csharpparser.IDeclaration_expressionContext) ssa.Value {
	i, _ := raw.(*csharpparser.Declaration_expressionContext)
	if i == nil {
		return nil
	}
	name := identText(i.Identifier())
	if name == "" {
		return b.EmitUndefined(i.GetText())
	}
	var value ssa.Value = b.EmitValueOnlyDeclare(name)
	typ := b.VisitLocalVariableType(i.Local_variable_type())
	variable := b.CreateLocalVariable(name)
	b.rememberDeclaredVariableType(variable, typ)
	value = b.applyDeclaredType(value, typ)
	b.AssignVariable(variable, value)
	return value
}

// ---------------------------------------------------------------- assignment

// visitAssignment compiles `left op= right`; left is a non_assignment_expression or unary_expression.
func (b *singleFileBuilder) visitAssignment(left antlr.ParserRuleContext, op csharpparser.IAssignment_operatorContext, rightExpr csharpparser.IExpressionContext) ssa.Value {
	oc, _ := op.(*csharpparser.Assignment_operatorContext)
	primary := b.unwrapToPrimary(left)

	// tuple deconstruction: (a, b) = ... / var (a, b) = ...
	if pe, _ := primary.(*csharpparser.Primary_expressionContext); pe != nil && pe.Tuple_expression() != nil && (oc == nil || oc.TK_EQ() != nil) {
		right := b.VisitExpression(rightExpr)
		if utils.IsNil(right) {
			right = b.EmitUndefined("tuple")
		}
		b.deconstructAssign(pe.Tuple_expression(), right)
		return right
	}

	variable := b.leftValueVariable(left, primary)
	visitRight := func() ssa.Value {
		right := b.VisitExpression(rightExpr)
		if !utils.IsNil(right) {
			return right
		}
		name := ""
		if variable != nil {
			name = variable.GetName()
		}
		return b.EmitUndefined(name)
	}
	if variable == nil {
		return visitRight()
	}
	if oc == nil || oc.TK_EQ() != nil {
		right := b.applyVariableDeclaredType(variable, visitRight())
		b.AssignVariable(variable, right)
		b.emitAssignmentSetter(primary, variable, right)
		return right
	}
	if oc.TK_QMARK_QMARK_EQ() != nil {
		cur := b.readAssignmentValue(primary, variable)
		result := b.emitNullCoalesce(cur, func() ssa.Value {
			right := visitRight()
			target := b.assignmentBranchVariable(variable)
			right = b.applyVariableDeclaredType(target, right)
			b.AssignVariable(target, right)
			b.emitAssignmentSetter(primary, target, right)
			return right
		})
		// Bind the merged value under the target's internal lookup name, but do
		// not carry its member-call marker: this updates subsequent reads without
		// manufacturing an unconditional field/indexer store after the branch.
		state := b.CreateVariableById(variable.GetName())
		result = b.applyVariableDeclaredType(state, result)
		b.AssignVariable(state, result)
		return result
	}
	cur := b.readAssignmentValue(primary, variable)
	if utils.IsNil(cur) {
		cur = b.EmitUndefined(variable.GetName())
	}
	right := visitRight()
	value := b.EmitBinOp(compoundAssignOpcode(oc), cur, right)
	value = b.applyVariableDeclaredType(variable, value)
	b.AssignVariable(variable, value)
	b.emitAssignmentSetter(primary, variable, value)
	return value
}

func compoundAssignOpcode(oc *csharpparser.Assignment_operatorContext) ssa.BinaryOpcode {
	switch {
	case oc.TK_PLUS_EQ() != nil:
		return ssa.OpAdd
	case oc.TK_MINUS_EQ() != nil:
		return ssa.OpSub
	case oc.TK_MUL_EQ() != nil:
		return ssa.OpMul
	case oc.TK_DIV_EQ() != nil:
		return ssa.OpDiv
	case oc.TK_PCT_EQ() != nil:
		return ssa.OpMod
	case oc.TK_AND_EQ() != nil:
		return ssa.OpAnd
	case oc.TK_OR_EQ() != nil:
		return ssa.OpOr
	case oc.TK_XOR_EQ() != nil:
		return ssa.OpXor
	case oc.TK_LT_LT_EQ() != nil:
		return ssa.OpShl
	case oc.Right_shift_assignment() != nil:
		return ssa.OpShr
	}
	return ssa.OpAdd
}

// VisitAssignment handles the statement-level `assignment: unary_expression assignment_operator expression`.
func (b *singleFileBuilder) VisitAssignment(raw csharpparser.IAssignmentContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.AssignmentContext)
	if !ok || i == nil {
		return nil
	}
	left, _ := i.Unary_expression().(*csharpparser.Unary_expressionContext)
	if left == nil {
		return b.VisitExpression(i.Expression())
	}
	return b.visitAssignment(left, i.Assignment_operator(), i.Expression())
}

// leftValueVariable resolves the assignment target to a variable.
func (b *singleFileBuilder) leftValueVariable(left antlr.ParserRuleContext, primary csharpparser.IPrimary_expressionContext) *ssa.Variable {
	if decl := b.unwrapToDeclaration(left); decl != nil {
		name := identText(decl.Identifier())
		if name != "" {
			return b.CreateVariable(name)
		}
	}
	if primary != nil {
		return b.VisitPrimaryLeftValue(primary)
	}
	if left == nil {
		return nil
	}
	return b.CreateVariable(left.GetText())
}

// assignmentBranchVariable creates the write version inside a conditional
// assignment branch without re-evaluating the already-resolved receiver or
// index. Creating a fresh version is important: assigning the pre-branch
// Variable directly would not be captured by IfBuilder's branch scope.
func (b *singleFileBuilder) assignmentBranchVariable(variable *ssa.Variable) *ssa.Variable {
	if variable == nil {
		return nil
	}
	target := b.CreateVariableById(variable.GetName())
	if variable.IsMemberCall() {
		obj, key := variable.GetMemberCall()
		target.SetMemberCall(obj, key)
	}
	return target
}

// unwrapToPrimary walks a pure pass-through chain down to the primary_expression.
// Returns nil if any level carries an operator (then it cannot be an lvalue).
func (b *singleFileBuilder) unwrapToPrimary(ctx antlr.Tree) csharpparser.IPrimary_expressionContext {
	for ctx != nil {
		switch c := ctx.(type) {
		case *csharpparser.ExpressionContext:
			if c.Assignment_operator() != nil {
				return nil
			}
			ctx = c.Non_assignment_expression()
		case *csharpparser.Non_assignment_expressionContext:
			if c.Conditional_expression() == nil {
				return nil
			}
			ctx = c.Conditional_expression()
		case *csharpparser.Conditional_expressionContext:
			if c.TK_QMARK() != nil {
				return nil
			}
			ctx = c.Null_coalescing_expression()
		case *csharpparser.Null_coalescing_expressionContext:
			if c.TK_QMARK_QMARK() != nil || c.Throw_expression() != nil {
				return nil
			}
			ctx = c.Conditional_or_expression()
		case *csharpparser.Conditional_or_expressionContext:
			if c.Conditional_or_expression() != nil {
				return nil
			}
			ctx = c.Conditional_and_expression()
		case *csharpparser.Conditional_and_expressionContext:
			if c.Conditional_and_expression() != nil {
				return nil
			}
			ctx = c.Inclusive_or_expression()
		case *csharpparser.Inclusive_or_expressionContext:
			if c.Inclusive_or_expression() != nil {
				return nil
			}
			ctx = c.Exclusive_or_expression()
		case *csharpparser.Exclusive_or_expressionContext:
			if c.Exclusive_or_expression() != nil {
				return nil
			}
			ctx = c.And_expression()
		case *csharpparser.And_expressionContext:
			if c.And_expression() != nil {
				return nil
			}
			ctx = c.Equality_expression()
		case *csharpparser.Equality_expressionContext:
			if c.Equality_expression() != nil {
				return nil
			}
			ctx = c.Relational_expression()
		case *csharpparser.Relational_expressionContext:
			if c.Relational_expression() != nil {
				return nil
			}
			ctx = c.Shift_expression()
		case *csharpparser.Shift_expressionContext:
			if c.Shift_expression() != nil {
				return nil
			}
			ctx = c.Additive_expression()
		case *csharpparser.Additive_expressionContext:
			if c.Additive_expression() != nil {
				return nil
			}
			ctx = c.Multiplicative_expression()
		case *csharpparser.Multiplicative_expressionContext:
			if c.Multiplicative_expression() != nil {
				return nil
			}
			ctx = c.Switch_expression()
		case *csharpparser.Switch_expressionContext:
			if c.KW_SWITCH() != nil {
				return nil
			}
			ctx = c.Range_expression()
		case *csharpparser.Range_expressionContext:
			if c.TK_DOT_DOT() != nil {
				return nil
			}
			us := c.AllUnary_expression()
			if len(us) != 1 {
				return nil
			}
			ctx = us[0]
		case *csharpparser.Unary_expressionContext:
			if c.Primary_expression() == nil {
				return nil
			}
			return c.Primary_expression()
		case *csharpparser.Primary_expressionContext:
			return c
		default:
			return nil
		}
	}
	return nil
}

// unwrapToDeclaration detects `var x` / `T x` used as an assignment target.
func (b *singleFileBuilder) unwrapToDeclaration(ctx antlr.Tree) *csharpparser.Declaration_expressionContext {
	switch c := ctx.(type) {
	case *csharpparser.ExpressionContext:
		if c.Assignment_operator() != nil {
			return nil
		}
		return b.unwrapToDeclaration(c.Non_assignment_expression())
	case *csharpparser.Non_assignment_expressionContext:
		d, _ := c.Declaration_expression().(*csharpparser.Declaration_expressionContext)
		return d
	}
	return nil
}

// ---------------------------------------------------------------- conditional / null-coalescing / logical

func (b *singleFileBuilder) VisitConditionalExpression(raw csharpparser.IConditional_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Conditional_expressionContext)
	if !ok || i == nil {
		return nil
	}
	cond := b.VisitNullCoalescing(i.Null_coalescing_expression())
	if i.TK_QMARK() == nil {
		return cond
	}
	exprs := i.AllExpression()
	refs := i.AllVariable_reference()
	var thenF, elseF func() ssa.Value
	switch {
	case len(exprs) >= 2:
		thenF = func() ssa.Value { return b.VisitExpression(exprs[0]) }
		elseF = func() ssa.Value { return b.VisitExpression(exprs[1]) }
	case len(refs) >= 2:
		thenF = func() ssa.Value { return b.VisitVariableReference(refs[0]) }
		elseF = func() ssa.Value { return b.VisitVariableReference(refs[1]) }
	default:
		return cond
	}
	if utils.IsNil(cond) {
		cond = b.EmitConstInst(true)
	}
	return b.emitTernary(cond, thenF, elseF)
}

// emitTernary builds `cond ? then : else` with a phi-merged temporary.
func (b *singleFileBuilder) emitTernary(cond ssa.Value, thenF, elseF func() ssa.Value) ssa.Value {
	id := ssa.TernaryExpressionVariable
	b.AssignVariable(b.CreateVariable(id), b.EmitValueOnlyDeclare(id))
	ifb := b.CreateIfBuilder()
	ifb.AppendItem(func() ssa.Value { return cond }, func() {
		v := thenF()
		if utils.IsNil(v) {
			v = b.EmitUndefined(id)
		}
		b.AssignVariable(b.CreateVariable(id), v)
	})
	ifb.SetElse(func() {
		v := elseF()
		if utils.IsNil(v) {
			v = b.EmitUndefined(id)
		}
		b.AssignVariable(b.CreateVariable(id), v)
	})
	ifb.Build()
	return b.ReadValue(id)
}

// emitNullCoalesce builds `left ?? right()`.
func (b *singleFileBuilder) emitNullCoalesce(left ssa.Value, rightF func() ssa.Value) ssa.Value {
	if utils.IsNil(left) {
		return rightF()
	}
	cond := b.EmitBinOp(ssa.OpNotEq, left, b.EmitConstInstNil())
	return b.emitTernary(cond, func() ssa.Value { return left }, rightF)
}

func (b *singleFileBuilder) VisitNullCoalescing(raw csharpparser.INull_coalescing_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Null_coalescing_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if te, _ := i.Throw_expression().(*csharpparser.Throw_expressionContext); te != nil {
		v := b.VisitNullCoalescing(te.Null_coalescing_expression())
		if utils.IsNil(v) {
			v = b.EmitUndefined("throw")
		}
		b.EmitPanic(v)
		return v
	}
	left := b.VisitConditionalOr(i.Conditional_or_expression())
	if i.TK_QMARK_QMARK() == nil || i.Null_coalescing_expression() == nil {
		return left
	}
	return b.emitNullCoalesce(left, func() ssa.Value {
		return b.VisitNullCoalescing(i.Null_coalescing_expression())
	})
}

func (b *singleFileBuilder) VisitConditionalOr(raw csharpparser.IConditional_or_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Conditional_or_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Conditional_or_expression() != nil {
		return b.binJump(
			func() ssa.Value { return b.VisitConditionalOr(i.Conditional_or_expression()) },
			func() ssa.Value { return b.VisitConditionalAnd(i.Conditional_and_expression()) },
			ssa.OrExpressionVariable, false)
	}
	return b.VisitConditionalAnd(i.Conditional_and_expression())
}

func (b *singleFileBuilder) VisitConditionalAnd(raw csharpparser.IConditional_and_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Conditional_and_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Conditional_and_expression() != nil {
		return b.binJump(
			func() ssa.Value { return b.VisitConditionalAnd(i.Conditional_and_expression()) },
			func() ssa.Value { return b.VisitInclusiveOr(i.Inclusive_or_expression()) },
			ssa.AndExpressionVariable, true)
	}
	return b.VisitInclusiveOr(i.Inclusive_or_expression())
}

// binJump implements short-circuit && / || with a phi-merged temporary.
func (b *singleFileBuilder) binJump(leftF, rightF func() ssa.Value, id string, and bool) ssa.Value {
	b.AssignVariable(b.CreateVariable(id), b.EmitValueOnlyDeclare(id))
	ifb := b.CreateIfBuilder()
	assignValue := func(v ssa.Value) {
		if utils.IsNil(v) {
			v = b.EmitUndefined(id)
		}
		b.AssignVariable(b.CreateVariable(id), v)
	}
	assign := func(f func() ssa.Value) func() {
		return func() { assignValue(f()) }
	}
	var leftBindings, rightBindings []patternBinding
	cond := func() ssa.Value {
		v := leftF()
		if and {
			leftBindings = b.patternBindingsFor(v)
		}
		if utils.IsNil(v) {
			v = b.EmitConstInst(true)
		}
		return v
	}
	if and {
		ifb.AppendItem(cond, func() {
			// A designation introduced by the left pattern is in scope for the
			// right operand of &&, but only on this successful edge.
			b.bindPatternBindings(leftBindings)
			right := rightF()
			rightBindings = b.patternBindingsFor(right)
			assignValue(right)
		})
		ifb.SetElse(assign(func() ssa.Value { return b.EmitConstInst(false) }))
	} else {
		ifb.AppendItem(cond, assign(func() ssa.Value { return b.EmitConstInst(true) }))
		ifb.SetElse(assign(rightF))
	}
	ifb.Build()
	result := b.ReadValue(id)
	if and {
		b.rememberPatternBindings(result, mergePatternBindings(leftBindings, rightBindings))
	}
	return result
}

// ---------------------------------------------------------------- bitwise / equality / relational

func (b *singleFileBuilder) binOp(op ssa.BinaryOpcode, left, right ssa.Value) ssa.Value {
	if utils.IsNil(left) {
		left = b.EmitUndefined("lhs")
	}
	if utils.IsNil(right) {
		right = b.EmitUndefined("rhs")
	}
	return b.EmitBinOp(op, left, right)
}

func (b *singleFileBuilder) VisitInclusiveOr(raw csharpparser.IInclusive_or_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Inclusive_or_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Inclusive_or_expression() != nil {
		return b.binOp(ssa.OpOr, b.VisitInclusiveOr(i.Inclusive_or_expression()), b.VisitExclusiveOr(i.Exclusive_or_expression()))
	}
	return b.VisitExclusiveOr(i.Exclusive_or_expression())
}

func (b *singleFileBuilder) VisitExclusiveOr(raw csharpparser.IExclusive_or_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Exclusive_or_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Exclusive_or_expression() != nil {
		return b.binOp(ssa.OpXor, b.VisitExclusiveOr(i.Exclusive_or_expression()), b.VisitAnd(i.And_expression()))
	}
	return b.VisitAnd(i.And_expression())
}

func (b *singleFileBuilder) VisitAnd(raw csharpparser.IAnd_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.And_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.And_expression() != nil {
		return b.binOp(ssa.OpAnd, b.VisitAnd(i.And_expression()), b.VisitEquality(i.Equality_expression()))
	}
	return b.VisitEquality(i.Equality_expression())
}

func (b *singleFileBuilder) VisitEquality(raw csharpparser.IEquality_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Equality_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Equality_expression() != nil {
		var op ssa.BinaryOpcode = ssa.OpEq
		if i.TK_NOT_EQ() != nil {
			op = ssa.OpNotEq
		}
		return b.binOp(op, b.VisitEquality(i.Equality_expression()), b.VisitRelational(i.Relational_expression()))
	}
	return b.VisitRelational(i.Relational_expression())
}

func (b *singleFileBuilder) VisitRelational(raw csharpparser.IRelational_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Relational_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Relational_expression() == nil {
		return b.VisitShift(i.Shift_expression())
	}
	left := b.VisitRelational(i.Relational_expression())
	if utils.IsNil(left) {
		left = b.EmitUndefined("lhs")
	}
	switch {
	case i.KW_IS() != nil:
		if i.Pattern() != nil {
			return b.emitPatternMatch(left, i.Pattern())
		}
		return b.emitIsType(left, b.VisitType(i.Type_()))
	case i.KW_AS() != nil:
		return b.emitAsType(left, b.VisitType(i.Type_()))
	}
	right := b.VisitShift(i.Shift_expression())
	switch {
	case i.TK_LT() != nil:
		return b.binOp(ssa.OpLt, left, right)
	case i.TK_GT() != nil:
		return b.binOp(ssa.OpGt, left, right)
	case i.TK_LT_EQ() != nil:
		return b.binOp(ssa.OpLtEq, left, right)
	case i.TK_GT_EQ() != nil:
		return b.binOp(ssa.OpGtEq, left, right)
	}
	return left
}

// emitIsType preserves a runtime `is` test as an explicit SSA call.  Reducing
// `x is T` to `x != nil` loses the tested type and produces false positives for
// every non-nil value, while a dedicated call keeps both operands available to
// SyntaxFlow and future type-aware passes.
func (b *singleFileBuilder) emitIsType(value ssa.Value, typ ssa.Type) ssa.Value {
	if utils.IsNil(value) {
		value = b.EmitUndefined("value")
	}
	typeName := "any"
	if typ != nil {
		typeName = typ.String()
	}
	callee := b.EmitUndefined("is")
	call := b.NewCall(callee, []ssa.Value{value, b.EmitConstInst(typeName)})
	result := b.EmitCall(call)
	result.SetType(ssa.CreateBooleanType())
	return result
}

// emitAsType approximates `x as T`: a type cast that keeps data flow.
func (b *singleFileBuilder) emitAsType(value ssa.Value, typ ssa.Type) ssa.Value {
	if utils.IsNil(value) {
		return b.EmitUndefined("as")
	}
	if typ == nil || typ.GetTypeKind() == ssa.AnyTypeKind {
		return value
	}
	return b.EmitTypeCast(value, typ)
}

func (b *singleFileBuilder) VisitShift(raw csharpparser.IShift_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Shift_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Shift_expression() != nil {
		var op ssa.BinaryOpcode = ssa.OpShl
		if i.Right_shift() != nil {
			op = ssa.OpShr
		}
		return b.binOp(op, b.VisitShift(i.Shift_expression()), b.VisitAdditive(i.Additive_expression()))
	}
	return b.VisitAdditive(i.Additive_expression())
}

func (b *singleFileBuilder) VisitAdditive(raw csharpparser.IAdditive_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Additive_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Additive_expression() != nil {
		var op ssa.BinaryOpcode = ssa.OpAdd
		if i.TK_MINUS() != nil {
			op = ssa.OpSub
		}
		return b.binOp(op, b.VisitAdditive(i.Additive_expression()), b.VisitMultiplicative(i.Multiplicative_expression()))
	}
	return b.VisitMultiplicative(i.Multiplicative_expression())
}

func (b *singleFileBuilder) VisitMultiplicative(raw csharpparser.IMultiplicative_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Multiplicative_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Multiplicative_expression() != nil {
		var op ssa.BinaryOpcode = ssa.OpMul
		switch {
		case i.SLASH() != nil:
			op = ssa.OpDiv
		case i.TK_PCT() != nil:
			op = ssa.OpMod
		}
		return b.binOp(op, b.VisitMultiplicative(i.Multiplicative_expression()), b.VisitSwitchExpression(i.Switch_expression()))
	}
	return b.VisitSwitchExpression(i.Switch_expression())
}

// ---------------------------------------------------------------- range / unary

func (b *singleFileBuilder) VisitRangeExpression(raw csharpparser.IRange_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Range_expressionContext)
	if !ok || i == nil {
		return nil
	}
	unis := i.AllUnary_expression()
	if i.TK_DOT_DOT() == nil {
		if len(unis) == 0 {
			return nil
		}
		return b.VisitUnaryExpression(unis[0])
	}
	// a..b → 容器 {0: a, 1: b}
	rng := b.EmitEmptyContainer()
	for idx, u := range unis {
		v := b.VisitUnaryExpression(u)
		if utils.IsNil(v) {
			continue
		}
		b.AssignVariable(b.CreateMemberCallVariable(rng, b.EmitConstInst(idx)), v)
	}
	return rng
}

func (b *singleFileBuilder) VisitUnaryExpression(raw csharpparser.IUnary_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Unary_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Primary_expression() != nil {
		return b.VisitPrimaryExpression(i.Primary_expression())
	}
	if i.Unary_expression() != nil {
		inner := b.VisitUnaryExpression(i.Unary_expression())
		if utils.IsNil(inner) {
			inner = b.EmitUndefined(i.Unary_expression().GetText())
		}
		switch {
		case i.TK_PLUS() != nil:
			return b.EmitUnOp(ssa.OpPlus, inner)
		case i.TK_MINUS() != nil:
			return b.EmitUnOp(ssa.OpNeg, inner)
		case i.Logical_negation_operator() != nil:
			return b.EmitUnOp(ssa.OpNot, inner)
		case i.TK_INV() != nil:
			return b.EmitUnOp(ssa.OpBitwiseNot, inner)
		case i.TK_XOR() != nil:
			// index-from-end ^n
			return inner
		}
		return inner
	}
	if ce, _ := i.Cast_expression().(*csharpparser.Cast_expressionContext); ce != nil {
		v := b.VisitUnaryExpression(ce.Unary_expression())
		if utils.IsNil(v) {
			return b.EmitUndefined(ce.GetText())
		}
		typ := b.VisitType(ce.Type_())
		if typ == nil || typ.GetTypeKind() == ssa.AnyTypeKind {
			return v
		}
		return b.EmitTypeCast(v, typ)
	}
	if pi, _ := i.Pre_increment_expression().(*csharpparser.Pre_increment_expressionContext); pi != nil {
		return b.visitIncDecUnary(pi.Unary_expression(), true, true)
	}
	if pd, _ := i.Pre_decrement_expression().(*csharpparser.Pre_decrement_expressionContext); pd != nil {
		return b.visitIncDecUnary(pd.Unary_expression(), false, true)
	}
	if ae, _ := i.Await_expression().(*csharpparser.Await_expressionContext); ae != nil {
		return b.VisitUnaryExpression(ae.Unary_expression())
	}
	if pe, _ := i.Pointer_indirection_expression().(*csharpparser.Pointer_indirection_expressionContext); pe != nil {
		return b.VisitUnaryExpression(pe.Unary_expression())
	}
	if ae, _ := i.Addressof_expression().(*csharpparser.Addressof_expressionContext); ae != nil {
		return b.VisitUnaryExpression(ae.Unary_expression())
	}
	return b.EmitUndefined(i.GetText())
}

// visitIncDecUnary handles prefix ++x / --x where x is a unary_expression.
func (b *singleFileBuilder) visitIncDecUnary(raw csharpparser.IUnary_expressionContext, inc, prefix bool) ssa.Value {
	primary := b.unwrapToPrimary(raw)
	if primary == nil {
		return b.VisitUnaryExpression(raw)
	}
	return b.visitIncDec(primary, inc, prefix)
}

// visitIncDec emits `x = x ± 1`; returns the new value for prefix, the old value for postfix.
func (b *singleFileBuilder) visitIncDec(target csharpparser.IPrimary_expressionContext, inc, prefix bool) ssa.Value {
	variable := b.VisitPrimaryLeftValue(target)
	if variable == nil {
		return b.VisitPrimaryExpression(target)
	}
	cur := b.readAssignmentValue(target, variable)
	if utils.IsNil(cur) {
		cur = b.EmitUndefined(variable.GetName())
	}
	var op ssa.BinaryOpcode = ssa.OpAdd
	if !inc {
		op = ssa.OpSub
	}
	next := b.EmitBinOp(op, cur, b.EmitConstInst(1))
	next = b.applyVariableDeclaredType(variable, next)
	b.AssignVariable(variable, next)
	b.emitAssignmentSetter(target, variable, next)
	if prefix {
		return next
	}
	return cur
}

// ---------------------------------------------------------------- arguments

type outArgument struct {
	index            int
	variable         *ssa.Variable
	calleeSideEffect bool
}

// csharpEvaluatedArgument separates source evaluation from call-vector
// binding. C# named arguments are evaluated exactly where they appear in the
// source, while the eventual SSA Call.Args must follow formal-parameter order.
type csharpEvaluatedArgument struct {
	value       ssa.Value
	name        string
	modifier    string
	outVariable *ssa.Variable
}

// VisitArgumentList evaluates arguments (positional; names ignored) and returns values only.
func (b *singleFileBuilder) VisitArgumentList(raw csharpparser.IArgument_listContext) []ssa.Value {
	values, _ := b.visitArguments(raw)
	return values
}

// visitArguments evaluates arguments and records `out`/`ref` targets for post-call binding.
func (b *singleFileBuilder) visitArguments(raw csharpparser.IArgument_listContext) ([]ssa.Value, []outArgument) {
	return flattenEvaluatedArguments(b.visitArgumentDetails(raw))
}

// visitArgumentDetails evaluates every argument once, in source order, and
// retains its optional name until a concrete callable has been selected.
func (b *singleFileBuilder) visitArgumentDetails(raw csharpparser.IArgument_listContext) []csharpEvaluatedArgument {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Argument_listContext)
	if !ok || i == nil {
		return nil
	}
	var arguments []csharpEvaluatedArgument
	for _, a := range i.AllArgument() {
		ac, _ := a.(*csharpparser.ArgumentContext)
		if ac == nil {
			continue
		}
		av, _ := ac.Argument_value().(*csharpparser.Argument_valueContext)
		if av == nil {
			continue
		}
		argument := csharpEvaluatedArgument{}
		if argumentName, _ := ac.Argument_name().(*csharpparser.Argument_nameContext); argumentName != nil {
			argument.name = identText(argumentName.Identifier())
		}
		switch {
		case av.KW_REF() != nil:
			argument.modifier = "ref"
		case av.KW_OUT() != nil:
			argument.modifier = "out"
		case av.KW_IN() != nil:
			argument.modifier = "in"
		}
		if av.Expression() != nil {
			v := b.VisitExpression(av.Expression())
			if utils.IsNil(v) {
				v = b.EmitUndefined(av.GetText())
			}
			argument.value = v
			arguments = append(arguments, argument)
			continue
		}
		vr, _ := av.Variable_reference().(*csharpparser.Variable_referenceContext)
		if vr == nil || vr.Expression() == nil {
			continue
		}
		if av.KW_IN() != nil {
			argument.value = b.VisitExpression(vr.Expression())
			if utils.IsNil(argument.value) {
				argument.value = b.EmitUndefined(vr.Expression().GetText())
			}
			arguments = append(arguments, argument)
			continue
		}
		expr := vr.Expression()
		variable := b.leftValueVariable(expr, b.unwrapToPrimary(expr))
		var cur ssa.Value
		if variable != nil {
			if av.KW_OUT() != nil {
				cur = b.EmitValueOnlyDeclare(variable.GetName())
			} else {
				cur = b.ReadValueByVariable(variable)
			}
			if utils.IsNil(cur) {
				cur = b.EmitUndefined(variable.GetName())
			}
			argument.outVariable = variable
		} else {
			cur = b.EmitUndefined(expr.GetText())
		}
		argument.value = cur
		arguments = append(arguments, argument)
	}
	return arguments
}

func flattenEvaluatedArguments(arguments []csharpEvaluatedArgument) ([]ssa.Value, []outArgument) {
	values := make([]ssa.Value, 0, len(arguments))
	var outs []outArgument
	for _, argument := range arguments {
		if utils.IsNil(argument.value) {
			continue
		}
		index := len(values)
		values = append(values, argument.value)
		if argument.outVariable != nil {
			outs = append(outs, outArgument{index: index, variable: argument.outVariable})
		}
	}
	return values, outs
}

// bindOutArguments makes `out`/`ref` variables depend on the call result.
func (b *singleFileBuilder) bindOutArguments(call ssa.Value, outs []outArgument) {
	if utils.IsNil(call) {
		return
	}
	for _, o := range outs {
		if o.variable == nil || o.calleeSideEffect {
			continue
		}
		v := b.ReadMemberCallValue(call, b.EmitConstInst(o.index))
		if utils.IsNil(v) {
			v = b.EmitUndefined(o.variable.GetName())
		}
		v = b.applyVariableDeclaredType(o.variable, v)
		b.AssignVariable(o.variable, v)
	}
}

func (b *singleFileBuilder) emitDetailedCall(callee ssa.Value, arguments []csharpEvaluatedArgument, fallbackName string) ssa.Value {
	if result, ok := b.emitAmbiguousDetailedMethodCall(callee, arguments, fallbackName, false); ok {
		return result
	}
	selected, args, outs := b.prepareDetailedCall(callee, arguments)
	result := b.emitCall(selected, args, outs, fallbackName)
	b.projectMethodExplicitWrites(result, selected, nil)
	return result
}

func (b *singleFileBuilder) emitDetailedBareCall(callee ssa.Value, class *ssa.Blueprint, name string, receiver ssa.Value, arguments []csharpEvaluatedArgument) ssa.Value {
	if result, ok := b.emitAmbiguousDetailedBareMethodCall(callee, class, name, receiver, arguments); ok {
		return result
	}
	selected, args, outs := b.prepareDetailedBareCall(callee, class, name, receiver, arguments)
	result := b.emitCall(selected, args, outs, name)
	b.projectMethodExplicitWrites(result, selected, receiver)
	return result
}

// emitCall is the single choke point for all call emission.
func (b *singleFileBuilder) emitCall(callee ssa.Value, args []ssa.Value, outs []outArgument, fallbackName string) ssa.Value {
	if utils.IsNil(callee) {
		callee = b.EmitUndefined(fallbackName)
	}
	call := b.EmitCall(b.NewCall(callee, args))
	if utils.IsNil(call) {
		return b.EmitUndefined(fallbackName)
	}
	b.projectReturnedConstructorState(call, callee)
	b.bindOutArguments(call, outs)
	return call
}
