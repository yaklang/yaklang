package csharp2ssa

import (
	"strconv"
	"strings"

	"github.com/yaklang/yaklang/common/utils"
	csharpparser "github.com/yaklang/yaklang/common/yak/csharp/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

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
	left := b.VisitNonAssignment(i.Non_assignment_expression())
	if i.Assignment_operator() == nil || i.Expression() == nil {
		return left
	}
	right := b.VisitExpression(i.Expression())
	return b.applyAssign(i.Non_assignment_expression().GetText(), i.Assignment_operator(), left, right)
}

func (b *singleFileBuilder) VisitNonAssignment(raw csharpparser.INon_assignment_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Non_assignment_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Conditional_expression() != nil {
		return b.VisitConditionalExpression(i.Conditional_expression())
	}
	if i.Lambda_expression() != nil {
		return b.EmitUndefined(i.GetText())
	}
	if i.Query_expression() != nil {
		return b.EmitUndefined(i.GetText())
	}
	if i.Declaration_expression() != nil {
		return b.EmitUndefined(i.GetText())
	}
	return nil
}

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
	if len(exprs) < 2 {
		return cond
	}
	ifb := b.CreateIfBuilder()
	id := ssa.TernaryExpressionVariable
	b.AssignVariable(b.CreateVariable(id), b.EmitValueOnlyDeclare(id))
	ifb.AppendItem(func() ssa.Value { return cond }, func() {
		v := b.VisitExpression(exprs[0])
		b.AssignVariable(b.CreateVariable(id), v)
	})
	ifb.SetElse(func() {
		v := b.VisitExpression(exprs[1])
		b.AssignVariable(b.CreateVariable(id), v)
	})
	ifb.Build()
	return b.ReadValue(id)
}

func (b *singleFileBuilder) VisitNullCoalescing(raw csharpparser.INull_coalescing_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Null_coalescing_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Throw_expression() != nil {
		te, _ := i.Throw_expression().(*csharpparser.Throw_expressionContext)
		if te != nil && te.Null_coalescing_expression() != nil {
			v := b.VisitNullCoalescing(te.Null_coalescing_expression())
			b.EmitPanic(v)
			return v
		}
		return b.EmitUndefined(i.GetText())
	}
	left := b.VisitConditionalOr(i.Conditional_or_expression())
	if i.TK_QMARK_QMARK() == nil || i.Null_coalescing_expression() == nil {
		return left
	}
	right := b.VisitNullCoalescing(i.Null_coalescing_expression())
	if left == nil {
		return right
	}
	if right == nil {
		return left
	}
	return left
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
		return b.binJump(func() ssa.Value { return b.VisitConditionalOr(i.Conditional_or_expression()) },
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
		return b.binJump(func() ssa.Value { return b.VisitConditionalAnd(i.Conditional_and_expression()) },
			func() ssa.Value { return b.VisitInclusiveOr(i.Inclusive_or_expression()) },
			ssa.AndExpressionVariable, true)
	}
	return b.VisitInclusiveOr(i.Inclusive_or_expression())
}

func (b *singleFileBuilder) binJump(leftF, rightF func() ssa.Value, name string, and bool) ssa.Value {
	id := name
	b.AssignVariable(b.CreateVariable(id), b.EmitValueOnlyDeclare(id))
	ifb := b.CreateIfBuilder()
	if and {
		ifb.AppendItem(func() ssa.Value { return leftF() }, func() {
			b.AssignVariable(b.CreateVariable(id), rightF())
		})
		ifb.SetElse(func() {
			b.AssignVariable(b.CreateVariable(id), b.EmitConstInst(false))
		})
	} else {
		ifb.AppendItem(func() ssa.Value { return leftF() }, func() {
			b.AssignVariable(b.CreateVariable(id), b.EmitConstInst(true))
		})
		ifb.SetElse(func() {
			b.AssignVariable(b.CreateVariable(id), rightF())
		})
	}
	ifb.Build()
	return b.ReadValue(id)
}

func (b *singleFileBuilder) VisitInclusiveOr(raw csharpparser.IInclusive_or_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Inclusive_or_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Inclusive_or_expression() != nil {
		return b.EmitBinOp(ssa.OpOr, b.VisitInclusiveOr(i.Inclusive_or_expression()), b.VisitExclusiveOr(i.Exclusive_or_expression()))
	}
	return b.VisitExclusiveOr(i.Exclusive_or_expression())
}

func (b *singleFileBuilder) VisitExclusiveOr(raw csharpparser.IExclusive_or_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Exclusive_or_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Exclusive_or_expression() != nil {
		return b.EmitBinOp(ssa.OpXor, b.VisitExclusiveOr(i.Exclusive_or_expression()), b.VisitAnd(i.And_expression()))
	}
	return b.VisitAnd(i.And_expression())
}

func (b *singleFileBuilder) VisitAnd(raw csharpparser.IAnd_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.And_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.And_expression() != nil {
		return b.EmitBinOp(ssa.OpAnd, b.VisitAnd(i.And_expression()), b.VisitEquality(i.Equality_expression()))
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
		return b.EmitBinOp(op, b.VisitEquality(i.Equality_expression()), b.VisitRelational(i.Relational_expression()))
	}
	return b.VisitRelational(i.Relational_expression())
}

func (b *singleFileBuilder) VisitRelational(raw csharpparser.IRelational_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Relational_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Relational_expression() != nil {
		left := b.VisitRelational(i.Relational_expression())
		if i.KW_IS() != nil || i.KW_AS() != nil {
			return left
		}
		right := b.VisitShift(i.Shift_expression())
		switch {
		case i.TK_LT() != nil:
			return b.EmitBinOp(ssa.OpLt, left, right)
		case i.TK_GT() != nil:
			return b.EmitBinOp(ssa.OpGt, left, right)
		case i.TK_LT_EQ() != nil:
			return b.EmitBinOp(ssa.OpLtEq, left, right)
		case i.TK_GT_EQ() != nil:
			return b.EmitBinOp(ssa.OpGtEq, left, right)
		}
		return left
	}
	return b.VisitShift(i.Shift_expression())
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
		return b.EmitBinOp(op, b.VisitShift(i.Shift_expression()), b.VisitAdditive(i.Additive_expression()))
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
		return b.EmitBinOp(op, b.VisitAdditive(i.Additive_expression()), b.VisitMultiplicative(i.Multiplicative_expression()))
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
		return b.EmitBinOp(op, b.VisitMultiplicative(i.Multiplicative_expression()), b.VisitSwitchExpression(i.Switch_expression()))
	}
	return b.VisitSwitchExpression(i.Switch_expression())
}

func (b *singleFileBuilder) VisitSwitchExpression(raw csharpparser.ISwitch_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Switch_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.KW_SWITCH() != nil {
		return b.VisitRangeExpression(i.Range_expression())
	}
	if i.Switch_expression() != nil {
		return b.VisitSwitchExpression(i.Switch_expression())
	}
	return b.VisitRangeExpression(i.Range_expression())
}

func (b *singleFileBuilder) VisitRangeExpression(raw csharpparser.IRange_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Range_expressionContext)
	if !ok || i == nil {
		return nil
	}
	unis := i.AllUnary_expression()
	if len(unis) == 0 {
		return nil
	}
	return b.VisitUnaryExpression(unis[0])
}

func (b *singleFileBuilder) VisitUnaryExpression(raw csharpparser.IUnary_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Unary_expressionContext)
	if !ok || i == nil {
		return nil
	}
	if i.Primary_expression() != nil {
		return b.VisitPrimaryExpression(i.Primary_expression())
	}
	if i.Unary_expression() != nil {
		inner := b.VisitUnaryExpression(i.Unary_expression())
		switch {
		case i.TK_PLUS() != nil:
			return b.EmitUnOp(ssa.OpPlus, inner)
		case i.TK_MINUS() != nil:
			return b.EmitUnOp(ssa.OpNeg, inner)
		case i.Logical_negation_operator() != nil:
			return b.EmitUnOp(ssa.OpNot, inner)
		case i.TK_INV() != nil:
			return b.EmitUnOp(ssa.OpBitwiseNot, inner)
		}
		return inner
	}
	if i.Cast_expression() != nil {
		ce, _ := i.Cast_expression().(*csharpparser.Cast_expressionContext)
		if ce != nil {
			v := b.VisitUnaryExpression(ce.Unary_expression())
			if ce.Type_() != nil && v != nil {
				v.SetType(b.VisitType(ce.Type_()))
			}
			return v
		}
	}
	if i.Pre_increment_expression() != nil {
		return b.VisitPreIncrement(i.Pre_increment_expression())
	}
	if i.Pre_decrement_expression() != nil {
		pd, _ := i.Pre_decrement_expression().(*csharpparser.Pre_decrement_expressionContext)
		if pd != nil {
			return b.visitPrefixUnary(pd.Unary_expression(), false)
		}
	}
	if i.Await_expression() != nil {
		ae, _ := i.Await_expression().(*csharpparser.Await_expressionContext)
		if ae != nil {
			return b.VisitUnaryExpression(ae.Unary_expression())
		}
	}
	return b.EmitUndefined(i.GetText())
}

func (b *singleFileBuilder) VisitPrimaryExpression(raw csharpparser.IPrimary_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw)
	defer recoverRange()
	i, ok := raw.(*csharpparser.Primary_expressionContext)
	if !ok || i == nil {
		return nil
	}

	if inner := i.Primary_expression(); inner != nil {
		obj := b.VisitPrimaryExpression(inner)
		if i.TK_LPAREN() != nil {
			args := b.VisitArgumentList(i.Argument_list())
			if utils.IsNil(obj) {
				obj = b.EmitUndefined(i.GetText())
			}
			call := b.NewCall(obj, args)
			return b.EmitCall(call)
		}
		if i.TK_DOT() != nil {
			key := identText(i.Identifier())
			if utils.IsNil(obj) {
				obj = b.ReadValue(key)
				if obj != nil {
					return obj
				}
				return b.EmitUndefined(i.GetText())
			}
			return b.ReadMemberCallValue(obj, b.EmitConstInst(key))
		}
		if i.TK_LBRACK() != nil {
			key := b.firstArgument(i.Argument_list())
			if utils.IsNil(obj) {
				return b.EmitUndefined(i.GetText())
			}
			if key == nil {
				key = b.EmitConstInst(0)
			}
			return b.ReadMemberCallValue(obj, key)
		}
		if i.TK_PLUS_PLUS() != nil {
			return b.visitPostfixValue(obj, inner, true)
		}
		if i.TK_MINUS_MINUS() != nil {
			return b.visitPostfixValue(obj, inner, false)
		}
		if i.TK_QMARK() != nil {
			return obj
		}
		return obj
	}

	if i.Literal() != nil {
		return b.VisitLiteral(i.Literal())
	}
	if i.Simple_name() != nil {
		sn, _ := i.Simple_name().(*csharpparser.Simple_nameContext)
		if sn != nil {
			name := identText(sn.Identifier())
			if bp := b.GetBluePrint(name); bp != nil {
				v := b.ReadValue(name)
				if v != nil {
					v.SetType(bp)
					return v
				}
				undef := b.EmitUndefined(name)
				undef.SetType(bp)
				return undef
			}
			return b.ReadValue(name)
		}
	}
	if i.Parenthesized_expression() != nil {
		pe, _ := i.Parenthesized_expression().(*csharpparser.Parenthesized_expressionContext)
		if pe != nil {
			return b.VisitExpression(pe.Expression())
		}
	}
	if i.This_access() != nil {
		return b.ReadValue("this")
	}
	if i.Object_creation_expression() != nil {
		return b.VisitObjectCreation(i.Object_creation_expression())
	}
	if i.Array_creation_expression() != nil {
		return b.EmitMakeWithoutType(nil, nil)
	}
	if i.Predefined_type() != nil && i.Identifier() != nil {
		obj := b.ReadValue(i.Predefined_type().GetText())
		return b.ReadMemberCallValue(obj, b.EmitConstInst(identText(i.Identifier())))
	}
	if i.Default_value_expression() != nil {
		return b.EmitUndefined("default")
	}
	if i.Typeof_expression() != nil {
		return b.EmitConstInst(i.GetText())
	}
	if i.Nameof_expression() != nil {
		return b.EmitConstInst(i.GetText())
	}
	if i.Interpolated_string_expression() != nil {
		return b.EmitConstInst(unquoteCSharpString(i.GetText()))
	}
	return b.EmitUndefined(i.GetText())
}

func (b *singleFileBuilder) VisitLiteral(raw csharpparser.ILiteralContext) ssa.Value {
	if b == nil || raw == nil {
		return nil
	}
	i, ok := raw.(*csharpparser.LiteralContext)
	if !ok || i == nil {
		return b.EmitUndefined("literal")
	}
	if i.Boolean_literal() != nil {
		bl, _ := i.Boolean_literal().(*csharpparser.Boolean_literalContext)
		if bl != nil && bl.TRUE() != nil {
			return b.EmitConstInst(true)
		}
		return b.EmitConstInst(false)
	}
	if i.Integer_Literal() != nil {
		text := strings.ReplaceAll(i.Integer_Literal().GetText(), "_", "")
		text = strings.TrimRight(strings.TrimRight(strings.TrimRight(text, "lL"), "uU"), "lL")
		if strings.HasPrefix(strings.ToLower(text), "0x") {
			if v, err := strconv.ParseInt(text[2:], 16, 64); err == nil {
				return b.EmitConstInst(v)
			}
		}
		if v, err := strconv.ParseInt(text, 10, 64); err == nil {
			return b.EmitConstInst(v)
		}
		return b.EmitConstInst(text)
	}
	if i.Real_Literal() != nil {
		text := strings.ReplaceAll(i.Real_Literal().GetText(), "_", "")
		text = strings.TrimRight(strings.TrimRight(strings.TrimRight(text, "fFdDmM"), "fFdDmM"), "fFdDmM")
		if v, err := strconv.ParseFloat(text, 64); err == nil {
			return b.EmitConstInst(v)
		}
		return b.EmitConstInst(text)
	}
	if i.String_Literal() != nil {
		return b.EmitConstInst(unquoteCSharpString(i.String_Literal().GetText()))
	}
	if i.Character_Literal() != nil {
		text := i.Character_Literal().GetText()
		if len(text) >= 3 {
			return b.EmitConstInst(text[1 : len(text)-1])
		}
		return b.EmitConstInst(text)
	}
	if i.Null_literal() != nil {
		return b.EmitConstInstNil()
	}
	return b.EmitUndefined(i.GetText())
}

func (b *singleFileBuilder) VisitObjectCreation(raw csharpparser.IObject_creation_expressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.Object_creation_expressionContext)
	if !ok || i == nil {
		return nil
	}
	typeName := ""
	if i.Type_() != nil {
		typeName = i.Type_().GetText()
	}
	args := b.VisitArgumentList(i.Argument_list())
	bp := b.GetBluePrint(typeName)
	if bp == nil && typeName != "" {
		bp = b.CreateBlueprint(typeName, i.Type_())
		bp.SetKind(ssa.BlueprintClass)
	}
	if bp != nil {
		b.ensureBlueprintConstructorSlot(bp)
		self := b.EmitUndefined(bp.Name)
		self.SetType(bp)
		callArgs := append([]ssa.Value{self}, args...)
		return b.ClassConstructorWithoutDeferDestructor(bp, callArgs)
	}
	callee := b.ReadValue(typeName)
	if callee == nil {
		callee = b.EmitUndefined(typeName)
	}
	return b.EmitCall(b.NewCall(callee, args))
}

func (b *singleFileBuilder) VisitAssignment(raw csharpparser.IAssignmentContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	i, ok := raw.(*csharpparser.AssignmentContext)
	if !ok || i == nil {
		return nil
	}
	right := b.VisitExpression(i.Expression())
	leftName := ""
	if i.Unary_expression() != nil {
		leftName = i.Unary_expression().GetText()
	}
	leftVal := b.VisitUnaryExpression(i.Unary_expression())
	return b.applyAssign(leftName, i.Assignment_operator(), leftVal, right)
}

func (b *singleFileBuilder) applyAssign(leftName string, op csharpparser.IAssignment_operatorContext, left, right ssa.Value) ssa.Value {
	if right == nil {
		right = b.EmitUndefined(leftName)
	}
	variable := b.leftVariable(leftName, left)
	if op != nil {
		oc, _ := op.(*csharpparser.Assignment_operatorContext)
		if oc != nil && oc.TK_EQ() == nil {
			cur := left
			if cur == nil {
				cur = b.ReadValue(leftName)
			}
			switch {
			case oc.TK_PLUS_EQ() != nil:
				right = b.EmitBinOp(ssa.OpAdd, cur, right)
			case oc.TK_MINUS_EQ() != nil:
				right = b.EmitBinOp(ssa.OpSub, cur, right)
			case oc.TK_MUL_EQ() != nil:
				right = b.EmitBinOp(ssa.OpMul, cur, right)
			case oc.TK_DIV_EQ() != nil:
				right = b.EmitBinOp(ssa.OpDiv, cur, right)
			case oc.TK_PCT_EQ() != nil:
				right = b.EmitBinOp(ssa.OpMod, cur, right)
			}
		}
	}
	if variable != nil {
		b.AssignVariable(variable, right)
	}
	return right
}

func (b *singleFileBuilder) leftVariable(name string, current ssa.Value) *ssa.Variable {
	if name == "" {
		return nil
	}
	if strings.Contains(name, ".") {
		parts := strings.Split(name, ".")
		objName := strings.Join(parts[:len(parts)-1], ".")
		key := parts[len(parts)-1]
		obj := b.ReadValue(objName)
		if obj == nil {
			obj = current
		}
		if obj == nil {
			return b.CreateVariable(name)
		}
		return b.CreateMemberCallVariable(obj, b.EmitConstInst(key))
	}
	if idx := strings.Index(name, "["); idx > 0 {
		objName := name[:idx]
		obj := b.ReadValue(objName)
		if obj == nil {
			return b.CreateVariable(name)
		}
		return b.CreateMemberCallVariable(obj, b.EmitConstInst(0))
	}
	return b.CreateVariable(name)
}

func (b *singleFileBuilder) VisitArgumentList(raw csharpparser.IArgument_listContext) []ssa.Value {
	if b == nil || raw == nil {
		return nil
	}
	i, ok := raw.(*csharpparser.Argument_listContext)
	if !ok || i == nil {
		return nil
	}
	var args []ssa.Value
	for _, a := range i.AllArgument() {
		ac, _ := a.(*csharpparser.ArgumentContext)
		if ac == nil || ac.Argument_value() == nil {
			continue
		}
		av, _ := ac.Argument_value().(*csharpparser.Argument_valueContext)
		if av == nil {
			continue
		}
		if av.Expression() != nil {
			args = append(args, b.VisitExpression(av.Expression()))
		} else if av.Variable_reference() != nil {
			args = append(args, b.EmitUndefined(av.GetText()))
		}
	}
	return args
}

func (b *singleFileBuilder) firstArgument(raw csharpparser.IArgument_listContext) ssa.Value {
	args := b.VisitArgumentList(raw)
	if len(args) == 0 {
		return nil
	}
	return args[0]
}

func (b *singleFileBuilder) VisitPostIncrement(raw csharpparser.IPost_increment_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Post_increment_expressionContext)
	if !ok || i == nil {
		return nil
	}
	return b.visitPostfix(i.Primary_expression(), true)
}

func (b *singleFileBuilder) VisitPreIncrement(raw csharpparser.IPre_increment_expressionContext) ssa.Value {
	i, ok := raw.(*csharpparser.Pre_increment_expressionContext)
	if !ok || i == nil {
		return nil
	}
	return b.visitPrefixUnary(i.Unary_expression(), true)
}

func (b *singleFileBuilder) visitPostfix(raw csharpparser.IPrimary_expressionContext, inc bool) ssa.Value {
	val := b.VisitPrimaryExpression(raw)
	return b.visitPostfixValue(val, raw, inc)
}

func (b *singleFileBuilder) visitPostfixValue(val ssa.Value, raw csharpparser.IPrimary_expressionContext, inc bool) ssa.Value {
	name := ""
	if raw != nil {
		name = raw.GetText()
	}
	variable := b.leftVariable(name, val)
	if variable == nil {
		return val
	}
	cur := b.ReadValueByVariable(variable)
	var next ssa.Value
	if inc {
		next = b.EmitBinOp(ssa.OpAdd, cur, b.EmitConstInst(1))
	} else {
		next = b.EmitBinOp(ssa.OpSub, cur, b.EmitConstInst(1))
	}
	b.AssignVariable(variable, next)
	return cur
}

func (b *singleFileBuilder) visitPrefixUnary(raw csharpparser.IUnary_expressionContext, inc bool) ssa.Value {
	name := ""
	if raw != nil {
		name = raw.GetText()
	}
	val := b.VisitUnaryExpression(raw)
	variable := b.leftVariable(name, val)
	if variable == nil {
		return val
	}
	cur := b.ReadValueByVariable(variable)
	var next ssa.Value
	if inc {
		next = b.EmitBinOp(ssa.OpAdd, cur, b.EmitConstInst(1))
	} else {
		next = b.EmitBinOp(ssa.OpSub, cur, b.EmitConstInst(1))
	}
	b.AssignVariable(variable, next)
	return next
}

func unquoteCSharpString(s string) string {
	if strings.HasPrefix(s, "@\"") && strings.HasSuffix(s, "\"") && len(s) >= 3 {
		inner := s[2 : len(s)-1]
		return strings.ReplaceAll(inner, `""`, `"`)
	}
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		if u, err := strconv.Unquote(s); err == nil {
			return u
		}
		return s[1 : len(s)-1]
	}
	return s
}
