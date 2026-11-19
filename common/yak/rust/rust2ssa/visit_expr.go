package rust2ssa

import (
	"fmt"
	"strconv"
	"strings"
	"sync/atomic"

	"github.com/yaklang/antlr/v4"

	rustparser "github.com/yaklang/yaklang/common/yak/rust/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (b *singleFileBuilder) VisitExpression(raw rustparser.IExpressionContext) ssa.Value {
	if b == nil || raw == nil || b.IsStop() {
		return nil
	}
	recoverRange := b.SetRange(raw.(antlr.ParserRuleContext))
	defer recoverRange()

	switch expr := raw.(type) {
	case *rustparser.LiteralExpression_Context:
		return b.visitLiteral(expr.LiteralExpression())
	case *rustparser.PathExpression_Context:
		return b.visitPath(expr.PathExpression())
	case *rustparser.CallExpressionContext:
		return b.visitCall(expr)
	case *rustparser.MethodCallExpressionContext:
		return b.visitMethodCall(expr)
	case *rustparser.FieldExpressionContext:
		return b.visitField(expr)
	case *rustparser.IndexExpressionContext:
		return b.visitIndex(expr)
	case *rustparser.AssignmentExpressionContext:
		return b.visitAssignment(expr)
	case *rustparser.CompoundAssignmentExpressionContext:
		return b.visitCompoundAssignment(expr)
	case *rustparser.ArithmeticOrLogicalExpressionContext:
		return b.visitArithmetic(expr)
	case *rustparser.ComparisonExpressionContext:
		return b.visitComparison(expr)
	case *rustparser.LazyBooleanExpressionContext:
		return b.visitLazyBoolean(expr)
	case *rustparser.NegationExpressionContext:
		return b.visitNegation(expr)
	case *rustparser.GroupedExpressionContext:
		return b.VisitExpression(expr.Expression())
	case *rustparser.BorrowExpressionContext:
		return b.VisitExpression(expr.Expression())
	case *rustparser.DereferenceExpressionContext:
		return b.VisitExpression(expr.Expression())
	case *rustparser.ReturnExpressionContext:
		return b.visitReturn(expr)
	case *rustparser.BreakExpressionContext:
		b.Break()
		return nil
	case *rustparser.ContinueExpressionContext:
		b.Continue()
		return nil
	case *rustparser.ExpressionWithBlock_Context:
		return b.visitExpressionWithBlock(expr.ExpressionWithBlock())
	case *rustparser.MacroInvocationAsExpressionContext:
		return b.visitMacro(expr.MacroInvocation())
	case *rustparser.StructExpression_Context:
		return b.visitStructExpression(expr.StructExpression())
	case *rustparser.ClosureExpression_Context:
		return b.visitClosure(expr.ClosureExpression())
	case *rustparser.TypeCastExpressionContext:
		return b.visitTypeCast(expr)
	case *rustparser.ArrayExpressionContext:
		return b.visitArrayExpression(expr)
	case *rustparser.TupleExpressionContext:
		return b.visitTupleExpression(expr)
	case *rustparser.TupleIndexingExpressionContext:
		receiver := b.VisitExpression(expr.Expression())
		index := ""
		if expr.TupleIndex() != nil {
			index = expr.TupleIndex().GetText()
		}
		return b.ReadMemberCallValue(receiver, b.EmitConstInst(index))
	default:
		// TODO(ssa-rust): unsupported expression forms are represented as Undefined instead of being lowered.
		return b.EmitUndefined(raw.GetText())
	}
}

func (b *singleFileBuilder) visitExpressionWithBlock(raw rustparser.IExpressionWithBlockContext) ssa.Value {
	ctx, ok := raw.(*rustparser.ExpressionWithBlockContext)
	if !ok || ctx == nil {
		return nil
	}
	switch {
	case ctx.BlockExpression() != nil:
		return b.visitBlockExpression(ctx.BlockExpression(), false)
	case ctx.IfExpression() != nil:
		if ifExpr, ok := ctx.IfExpression().(*rustparser.IfExpressionContext); ok {
			return b.visitIfExpression(ifExpr)
		}
	case ctx.LoopExpression() != nil:
		b.visitLoop(ctx.LoopExpression())
	case ctx.MatchExpression() != nil:
		return b.visitMatch(ctx.MatchExpression())
	}
	return nil
}

var rustIfExpressionSeq atomic.Uint64

// visitIfExpression lowers Rust's value-producing if expression. The IfBuilder
// only merges control flow, so branch values are written into a temporary
// variable and read after the merge, where the builder materializes its phi.
func (b *singleFileBuilder) visitIfExpression(ctx *rustparser.IfExpressionContext) ssa.Value {
	if ctx == nil || ctx.Expression() == nil {
		return nil
	}
	name := fmt.Sprintf("__rust_if_expr_%d", rustIfExpressionSeq.Add(1))
	variable := b.CreateLocalVariable(name)
	b.AssignVariable(variable, b.EmitConstInstNil())
	ifBuilder := b.CreateIfBuilder()
	ifBuilder.SetCondition(func() ssa.Value {
		return b.VisitExpression(ctx.Expression())
	}, func() {
		value := b.visitBlockExpression(ctx.BlockExpression(0), false)
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateVariable(name), value)
	})
	if ctx.IfExpression() != nil {
		if elseIf, ok := ctx.IfExpression().(*rustparser.IfExpressionContext); ok {
			ifBuilder.SetElse(func() {
				value := b.visitIfExpression(elseIf)
				if value == nil {
					value = b.EmitConstInstNil()
				}
				b.AssignVariable(b.CreateVariable(name), value)
			})
		}
	} else if ctx.BlockExpression(1) != nil {
		ifBuilder.SetElse(func() {
			value := b.visitBlockExpression(ctx.BlockExpression(1), false)
			if value == nil {
				value = b.EmitConstInstNil()
			}
			b.AssignVariable(b.CreateVariable(name), value)
		})
	}
	ifBuilder.Build()
	return b.ReadValue(name)
}

func (b *singleFileBuilder) visitLiteral(raw rustparser.ILiteralExpressionContext) ssa.Value {
	literal, ok := raw.(*rustparser.LiteralExpressionContext)
	if !ok || literal == nil {
		return b.EmitConstInstNil()
	}
	switch {
	case literal.KW_TRUE() != nil:
		return b.EmitConstInst(true)
	case literal.KW_FALSE() != nil:
		return b.EmitConstInst(false)
	case literal.INTEGER_LITERAL() != nil:
		text := trimRustIntSuffix(literal.INTEGER_LITERAL().GetText())
		if value, err := strconv.ParseInt(text, 0, 64); err == nil {
			return b.EmitConstInst(value)
		}
		return b.EmitConstInst(text)
	case literal.FLOAT_LITERAL() != nil:
		text := trimRustFloatSuffix(literal.FLOAT_LITERAL().GetText())
		if value, err := strconv.ParseFloat(text, 64); err == nil {
			return b.EmitConstInst(value)
		}
		return b.EmitConstInst(text)
	case literal.STRING_LITERAL() != nil:
		return b.EmitConstInst(unquoteRustString(literal.STRING_LITERAL().GetText()))
	case literal.RAW_STRING_LITERAL() != nil:
		return b.EmitConstInst(unquoteRustString(literal.RAW_STRING_LITERAL().GetText()))
	case literal.BYTE_STRING_LITERAL() != nil:
		return b.EmitConstInst(unquoteRustString(literal.BYTE_STRING_LITERAL().GetText()))
	case literal.RAW_BYTE_STRING_LITERAL() != nil:
		return b.EmitConstInst(unquoteRustString(literal.RAW_BYTE_STRING_LITERAL().GetText()))
	case literal.CHAR_LITERAL() != nil:
		return b.EmitConstInst(strings.Trim(literal.CHAR_LITERAL().GetText(), "'"))
	case literal.BYTE_LITERAL() != nil:
		return b.EmitConstInst(strings.Trim(literal.BYTE_LITERAL().GetText(), "'"))
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitPath(raw rustparser.IPathExpressionContext) ssa.Value {
	path, ok := raw.(*rustparser.PathExpressionContext)
	if !ok || path == nil {
		return b.EmitUndefined("path")
	}
	name := rustPathName(path)
	if name == "" {
		return b.EmitUndefined("path")
	}
	value := b.ReadValue(name)
	if value == nil {
		value = b.EmitUndefined(name)
	}
	return value
}

func (b *singleFileBuilder) visitCall(ctx *rustparser.CallExpressionContext) ssa.Value {
	callee := b.VisitExpression(ctx.Expression())
	if callee == nil {
		callee = b.EmitUndefined("call")
	}
	args := b.visitCallParams(ctx.CallParams())
	return b.EmitCall(b.NewCall(callee, args))
}

func (b *singleFileBuilder) visitMethodCall(ctx *rustparser.MethodCallExpressionContext) ssa.Value {
	receiver := b.VisitExpression(ctx.Expression())
	if receiver == nil {
		receiver = b.EmitUndefined("receiver")
	}
	name := ""
	if ctx.PathExprSegment() != nil {
		name = ctx.PathExprSegment().GetText()
	}
	method := b.ReadMemberCallMethod(receiver, b.EmitConstInst(name))
	args := b.visitCallParams(ctx.CallParams())
	if method == nil {
		method = b.EmitUndefined(name)
	}
	// Inherent impl methods are registered as plain functions with an
	// explicit self parameter. Bind the receiver as the first argument so
	// `self.field` resolves in the method body.
	if _, ok := ssa.ToFunction(method); !ok {
		if fn := b.ReadValue(name); fn != nil {
			if _, isFn := ssa.ToFunction(fn); isFn {
				method = fn
				args = append([]ssa.Value{receiver}, args...)
			}
		}
	}
	return b.EmitCall(b.NewCall(method, args))
}

func (b *singleFileBuilder) visitField(ctx *rustparser.FieldExpressionContext) ssa.Value {
	receiver := b.VisitExpression(ctx.Expression())
	if receiver == nil {
		receiver = b.EmitUndefined("receiver")
	}
	name := ""
	if ctx.Identifier() != nil {
		name = ctx.Identifier().GetText()
	}
	return b.ReadMemberCallValue(receiver, b.EmitConstInst(name))
}

func (b *singleFileBuilder) visitIndex(ctx *rustparser.IndexExpressionContext) ssa.Value {
	receiver := b.VisitExpression(ctx.Expression(0))
	if receiver == nil {
		receiver = b.EmitUndefined("receiver")
	}
	index := b.VisitExpression(ctx.Expression(1))
	if index == nil {
		index = b.EmitConstInstNil()
	}
	return b.ReadMemberCallValue(receiver, index)
}

func (b *singleFileBuilder) visitAssignment(ctx *rustparser.AssignmentExpressionContext) ssa.Value {
	left := ctx.Expression(0)
	value := b.VisitExpression(ctx.Expression(1))
	if value == nil {
		value = b.EmitConstInstNil()
	}
	b.assignToExpression(left, value)
	return value
}

func (b *singleFileBuilder) visitCompoundAssignment(ctx *rustparser.CompoundAssignmentExpressionContext) ssa.Value {
	left := ctx.Expression(0)
	value := b.VisitExpression(ctx.Expression(1))
	if value == nil {
		value = b.EmitConstInstNil()
	}
	current := b.VisitExpression(left)
	if current == nil {
		current = b.EmitConstInstNil()
	}
	operator := ""
	if ctx.CompoundAssignOperator() != nil {
		operator = strings.TrimSuffix(ctx.CompoundAssignOperator().GetText(), "=")
	}
	result := b.EmitBinOp(rustBinaryOpcode(operator), current, value)
	b.assignToExpression(left, result)
	return result
}

func (b *singleFileBuilder) visitArithmetic(ctx *rustparser.ArithmeticOrLogicalExpressionContext) ssa.Value {
	left := b.VisitExpression(ctx.Expression(0))
	right := b.VisitExpression(ctx.Expression(1))
	if left == nil {
		left = b.EmitConstInstNil()
	}
	if right == nil {
		right = b.EmitConstInstNil()
	}
	operator := ""
	switch {
	case ctx.STAR() != nil:
		operator = "*"
	case ctx.SLASH() != nil:
		operator = "/"
	case ctx.PERCENT() != nil:
		operator = "%"
	case ctx.PLUS() != nil:
		operator = "+"
	case ctx.MINUS() != nil:
		operator = "-"
	case ctx.Shl() != nil:
		operator = "<<"
	case ctx.Shr() != nil:
		operator = ">>"
	case ctx.AND() != nil:
		operator = "&"
	case ctx.CARET() != nil:
		operator = "^"
	case ctx.OR() != nil:
		operator = "|"
	}
	return b.EmitBinOp(rustBinaryOpcode(operator), left, right)
}

func (b *singleFileBuilder) visitComparison(ctx *rustparser.ComparisonExpressionContext) ssa.Value {
	left := b.VisitExpression(ctx.Expression(0))
	right := b.VisitExpression(ctx.Expression(1))
	if left == nil {
		left = b.EmitConstInstNil()
	}
	if right == nil {
		right = b.EmitConstInstNil()
	}
	operator := ""
	if ctx.ComparisonOperator() != nil {
		operator = ctx.ComparisonOperator().GetText()
	}
	return b.EmitBinOp(rustBinaryOpcode(operator), left, right)
}

func (b *singleFileBuilder) visitLazyBoolean(ctx *rustparser.LazyBooleanExpressionContext) ssa.Value {
	return b.BuildShortCircuit(
		func() ssa.Value { return b.VisitExpression(ctx.Expression(0)) },
		func() ssa.Value { return b.VisitExpression(ctx.Expression(1)) },
		ctx.OROR() == nil,
	)
}

func (b *singleFileBuilder) visitNegation(ctx *rustparser.NegationExpressionContext) ssa.Value {
	value := b.VisitExpression(ctx.Expression())
	if value == nil {
		value = b.EmitConstInstNil()
	}
	if ctx.NOT() != nil {
		return b.EmitUnOp(ssa.OpNot, value)
	}
	return b.EmitUnOp(ssa.OpNeg, value)
}

func (b *singleFileBuilder) visitReturn(ctx *rustparser.ReturnExpressionContext) ssa.Value {
	values := []ssa.Value{}
	if ctx.Expression() != nil {
		if value := b.VisitExpression(ctx.Expression()); value != nil {
			values = append(values, value)
		}
	}
	b.EmitReturn(values)
	return nil
}

func (b *singleFileBuilder) visitIf(ctx *rustparser.IfExpressionContext) {
	if ctx.Expression() == nil {
		return
	}
	ifBuilder := b.CreateIfBuilder()
	ifBuilder.SetCondition(func() ssa.Value {
		return b.VisitExpression(ctx.Expression())
	}, func() {
		if ctx.BlockExpression(0) != nil {
			b.visitBlockExpression(ctx.BlockExpression(0), false)
		}
	})
	if ctx.IfExpression() != nil {
		if elseIf, ok := ctx.IfExpression().(*rustparser.IfExpressionContext); ok {
			ifBuilder.SetElse(func() {
				b.visitIf(elseIf)
			})
		}
	} else if ctx.BlockExpression(1) != nil {
		ifBuilder.SetElse(func() {
			b.visitBlockExpression(ctx.BlockExpression(1), false)
		})
	}
	ifBuilder.Build()
}

func (b *singleFileBuilder) visitLoop(raw rustparser.ILoopExpressionContext) {
	loopCtx, ok := raw.(*rustparser.LoopExpressionContext)
	if !ok || loopCtx == nil {
		return
	}
	loop := b.CreateLoopBuilder()
	switch {
	case loopCtx.InfiniteLoopExpression() != nil:
		infinite, ok := loopCtx.InfiniteLoopExpression().(*rustparser.InfiniteLoopExpressionContext)
		if !ok || infinite == nil {
			return
		}
		loop.SetCondition(func() ssa.Value { return b.EmitConstInst(true) })
		loop.SetBody(func() {
			b.visitBlockExpression(infinite.BlockExpression(), false)
		})
	case loopCtx.PredicateLoopExpression() != nil:
		predicate, ok := loopCtx.PredicateLoopExpression().(*rustparser.PredicateLoopExpressionContext)
		if !ok || predicate == nil {
			return
		}
		loop.SetCondition(func() ssa.Value {
			return b.VisitExpression(predicate.Expression())
		})
		loop.SetBody(func() {
			b.visitBlockExpression(predicate.BlockExpression(), false)
		})
	case loopCtx.IteratorLoopExpression() != nil:
		iterator, ok := loopCtx.IteratorLoopExpression().(*rustparser.IteratorLoopExpressionContext)
		if !ok || iterator == nil {
			return
		}
		name := rustPatternName(iterator.Pattern())
		loop.SetFirst(func() []ssa.Value {
			if name != "" {
				b.AssignVariable(b.CreateLocalVariable(name), b.EmitUndefined(name))
			}
			return nil
		})
		loop.SetCondition(func() ssa.Value { return b.EmitConstInst(true) })
		loop.SetBody(func() {
			b.visitBlockExpression(iterator.BlockExpression(), false)
		})
	default:
		return
	}
	loop.Finish()
}

func (b *singleFileBuilder) visitMacro(raw rustparser.IMacroInvocationContext) ssa.Value {
	macro, ok := raw.(*rustparser.MacroInvocationContext)
	if !ok || macro == nil {
		return nil
	}
	name := "macro"
	if macro.SimplePath() != nil {
		name = macro.SimplePath().GetText()
	}
	target := b.ReadValue(name)
	if target == nil {
		target = b.EmitUndefined(name)
	}
	// TODO(ssa-rust): macro token trees are kept as strings; expansion and hygiene are not modeled.
	args := []ssa.Value{}
	if macro.DelimTokenTree() != nil {
		args = append(args, b.EmitConstInst(macro.DelimTokenTree().GetText()))
	}
	return b.EmitCall(b.NewCall(target, args))
}

func (b *singleFileBuilder) visitCallParams(raw rustparser.ICallParamsContext) []ssa.Value {
	params, ok := raw.(*rustparser.CallParamsContext)
	if !ok || params == nil {
		return nil
	}
	values := make([]ssa.Value, 0, len(params.AllExpression()))
	for _, expr := range params.AllExpression() {
		values = append(values, b.VisitExpression(expr))
	}
	return values
}

func (b *singleFileBuilder) assignToExpression(raw rustparser.IExpressionContext, value ssa.Value) {
	if value == nil {
		value = b.EmitConstInstNil()
	}
	switch left := raw.(type) {
	case *rustparser.PathExpression_Context:
		name := rustPathName(left.PathExpression())
		if name != "" {
			b.AssignVariable(b.CreateVariable(name), value)
		}
	case *rustparser.FieldExpressionContext:
		object := b.VisitExpression(left.Expression())
		key := ""
		if left.Identifier() != nil {
			key = left.Identifier().GetText()
		}
		if object != nil {
			b.AssignVariable(b.CreateMemberCallVariable(object, b.EmitConstInst(key)), value)
		}
	case *rustparser.IndexExpressionContext:
		object := b.VisitExpression(left.Expression(0))
		index := b.VisitExpression(left.Expression(1))
		if object != nil && index != nil {
			b.AssignVariable(b.CreateMemberCallVariable(object, index), value)
		}
	case *rustparser.GroupedExpressionContext:
		b.assignToExpression(left.Expression(), value)
	}
}

func rustPathName(raw rustparser.IPathExpressionContext) string {
	path, ok := raw.(*rustparser.PathExpressionContext)
	if !ok || path == nil || path.PathInExpression() == nil {
		return ""
	}
	inExpression, ok := path.PathInExpression().(*rustparser.PathInExpressionContext)
	if !ok || inExpression == nil || len(inExpression.AllPathExprSegment()) == 0 {
		return ""
	}
	segments := inExpression.AllPathExprSegment()
	last := segments[len(segments)-1]
	if last == nil || last.PathIdentSegment() == nil {
		return ""
	}
	return last.PathIdentSegment().GetText()
}

func rustBinaryOpcode(operator string) ssa.BinaryOpcode {
	switch operator {
	case "+":
		return ssa.OpAdd
	case "-":
		return ssa.OpSub
	case "*":
		return ssa.OpMul
	case "/":
		return ssa.OpDiv
	case "%":
		return ssa.OpMod
	case "<<":
		return ssa.OpShl
	case ">>":
		return ssa.OpShr
	case "&":
		return ssa.OpAnd
	case "|":
		return ssa.OpOr
	case "^":
		return ssa.OpXor
	case "==":
		return ssa.OpEq
	case "!=":
		return ssa.OpNotEq
	case "<":
		return ssa.OpLt
	case ">":
		return ssa.OpGt
	case "<=":
		return ssa.OpLtEq
	case ">=":
		return ssa.OpGtEq
	case "&&":
		return ssa.OpAnd
	case "||":
		return ssa.OpOr
	default:
		return ssa.OpAdd
	}
}

func trimRustIntSuffix(text string) string {
	text = strings.ReplaceAll(text, "_", "")
	for _, suffix := range []string{"isize", "usize", "i128", "u128", "i64", "u64", "i32", "u32", "i16", "u16", "i8", "u8"} {
		if strings.HasSuffix(text, suffix) {
			return strings.TrimSuffix(text, suffix)
		}
	}
	return text
}

func trimRustFloatSuffix(text string) string {
	text = strings.ReplaceAll(text, "_", "")
	for _, suffix := range []string{"f32", "f64"} {
		if strings.HasSuffix(text, suffix) {
			return strings.TrimSuffix(text, suffix)
		}
	}
	return text
}

func unquoteRustString(text string) string {
	text = strings.TrimPrefix(text, "b")
	text = strings.TrimPrefix(text, "r")
	text = strings.Trim(text, "#")
	text = strings.Trim(text, "\"")
	return text
}

func (b *singleFileBuilder) visitTypeCast(ctx *rustparser.TypeCastExpressionContext) ssa.Value {
	// TODO(ssa-rust): `as` has no runtime check; pointer/reference and user-defined casts collapse to TypeCast.
	value := b.VisitExpression(ctx.Expression())
	if value == nil {
		value = b.EmitConstInstNil()
	}
	typ := rustTypeFromText("")
	if ctx.TypeNoBounds() != nil {
		typ = rustTypeFromText(ctx.TypeNoBounds().GetText())
	}
	return b.EmitTypeCast(value, typ)
}

func rustTypeFromText(text string) ssa.Type {
	text = strings.TrimSpace(text)
	for strings.HasPrefix(text, "&") || strings.HasPrefix(text, "*") {
		text = strings.TrimSpace(strings.TrimLeft(text, "&*"))
	}
	switch text {
	case "f32", "f64":
		return ssa.CreateFloatType()
	case "i8", "i16", "i32", "i64", "i128", "isize",
		"u8", "u16", "u32", "u64", "u128", "usize":
		return ssa.CreateNumberType()
	case "bool":
		return ssa.CreateBooleanType()
	case "str", "String", "char":
		return ssa.CreateStringType()
	default:
		return ssa.CreateAnyType()
	}
}

func (b *singleFileBuilder) visitClosure(raw rustparser.IClosureExpressionContext) ssa.Value {
	ctx, ok := raw.(*rustparser.ClosureExpressionContext)
	if !ok || ctx == nil {
		return b.EmitConstInstNil()
	}
	previous := b.FunctionBuilder
	previousSupport := previous.SupportClosure
	previous.SupportClosure = true
	// TODO(ssa-rust): move/borrow capture, upvalue mutation, and lifetimes are not modeled.
	fn := previous.NewFunc("")
	b.FunctionBuilder = previous.PushFunction(fn)
	b.SupportClosure = true
	b.SetForceCapture(true)
	b.LoadGlobalVariable()
	if params := ctx.ClosureParameters(); params != nil {
		if list, ok := params.(*rustparser.ClosureParametersContext); ok && list != nil {
			for _, rawParam := range list.AllClosureParam() {
				param, ok := rawParam.(*rustparser.ClosureParamContext)
				if !ok || param == nil {
					continue
				}
				if name := rustPatternName(param.Pattern()); name != "" {
					b.NewParam(name)
				} else {
					b.NewParam("_")
				}
			}
		}
	}
	if ctx.Expression() != nil {
		if value := b.VisitExpression(ctx.Expression()); value != nil {
			b.EmitReturn([]ssa.Value{value})
		}
	} else if ctx.BlockExpression() != nil {
		if value := b.visitBlockExpression(ctx.BlockExpression(), false); value != nil {
			b.EmitReturn([]ssa.Value{value})
		}
	}
	b.Finish()
	b.SetForceCapture(false)
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	previous.SupportClosure = previousSupport
	return fn
}

func (b *singleFileBuilder) visitMatch(raw rustparser.IMatchExpressionContext) ssa.Value {
	// TODO(ssa-rust): match exhaustiveness, enum variants, and destructuring patterns are not modeled.
	match, ok := raw.(*rustparser.MatchExpressionContext)
	if !ok || match == nil || match.MatchArms() == nil {
		return b.EmitConstInstNil()
	}
	subject := b.VisitExpression(match.Expression())
	if subject == nil {
		subject = b.EmitConstInstNil()
	}
	arms, ok := match.MatchArms().(*rustparser.MatchArmsContext)
	if !ok || arms == nil {
		return b.EmitConstInstNil()
	}
	allArms := arms.AllMatchArm()
	expressions := arms.AllMatchArmExpression()
	cases := make([]ssa.MatchCase, 0, len(allArms))
	for i, rawArm := range allArms {
		arm, ok := rawArm.(*rustparser.MatchArmContext)
		if !ok || arm == nil {
			continue
		}
		condition, bindings := b.rustPatternCondition(subject, arm.Pattern())
		guard := arm.MatchArmGuard()
		body := func() ssa.Value {
			if i < len(expressions) {
				if expr, ok := expressions[i].(*rustparser.MatchArmExpressionContext); ok && expr != nil {
					if expr.Expression() != nil {
						return b.VisitExpression(expr.Expression())
					}
					if expr.ExpressionWithBlock() != nil {
						return b.visitExpressionWithBlock(expr.ExpressionWithBlock())
					}
				}
			}
			if arms.Expression() != nil {
				return b.VisitExpression(arms.Expression())
			}
			return b.EmitConstInstNil()
		}
		cases = append(cases, ssa.MatchCase{
			Condition: func() ssa.Value {
				for _, binding := range bindings {
					b.AssignVariable(b.CreateVariable(binding), subject)
				}
				result := condition
				if result == nil {
					result = b.EmitConstInst(true)
				}
				if guard != nil {
					if guardCtx, ok := guard.(*rustparser.MatchArmGuardContext); ok && guardCtx.Expression() != nil {
						if value := b.VisitExpression(guardCtx.Expression()); value != nil {
							result = b.EmitBinOp(ssa.OpAnd, result, value)
						}
					}
				}
				return result
			},
			Body: body,
		})
	}
	return b.BuildMatchValue(cases, nil)
}

func (b *singleFileBuilder) rustPatternCondition(subject ssa.Value, raw rustparser.IPatternContext) (ssa.Value, []string) {
	pattern, ok := raw.(*rustparser.PatternContext)
	if !ok || pattern == nil || len(pattern.AllPatternNoTopAlt()) == 0 {
		return b.EmitConstInst(true), nil
	}
	var result ssa.Value
	var bindings []string
	for _, rawAlt := range pattern.AllPatternNoTopAlt() {
		alt, ok := rawAlt.(*rustparser.PatternNoTopAltContext)
		if !ok || alt == nil {
			continue
		}
		condition, names := b.rustPatternNoTopAltCondition(subject, alt)
		bindings = append(bindings, names...)
		if result == nil {
			result = condition
		} else {
			result = b.EmitBinOp(ssa.OpOr, result, condition)
		}
	}
	if result == nil {
		result = b.EmitConstInst(true)
	}
	return result, bindings
}

func (b *singleFileBuilder) rustPatternNoTopAltCondition(subject ssa.Value, alt *rustparser.PatternNoTopAltContext) (ssa.Value, []string) {
	if alt.RangePattern() != nil {
		return b.rustRangePatternCondition(subject, alt.RangePattern().GetText()), nil
	}
	without, ok := alt.PatternWithoutRange().(*rustparser.PatternWithoutRangeContext)
	if !ok || without == nil {
		return b.EmitConstInst(true), nil
	}
	switch {
	case without.WildcardPattern() != nil || without.RestPattern() != nil:
		return b.EmitConstInst(true), nil
	case without.LiteralPattern() != nil:
		if value := b.rustLiteralPatternValue(without.LiteralPattern()); value != nil {
			return b.EmitBinOp(ssa.OpEq, subject, value), nil
		}
		return b.EmitConstInst(true), nil
	case without.IdentifierPattern() != nil:
		identifier, ok := without.IdentifierPattern().(*rustparser.IdentifierPatternContext)
		if !ok || identifier == nil || identifier.Identifier() == nil {
			return b.EmitConstInst(true), nil
		}
		name := identifier.Identifier().GetText()
		if name == "_" {
			return b.EmitConstInst(true), nil
		}
		return b.EmitConstInst(true), []string{name}
	case without.GroupedPattern() != nil:
		grouped, ok := without.GroupedPattern().(*rustparser.GroupedPatternContext)
		if ok && grouped != nil {
			return b.rustPatternCondition(subject, grouped.Pattern())
		}
	case without.PathPattern() != nil:
		if strings.HasSuffix(without.PathPattern().GetText(), "None") {
			return b.EmitBinOp(ssa.OpEq, subject, b.EmitConstInstNil()), nil
		}
		// TODO(ssa-rust): non-None path patterns (enum variants/constants) become a catch-all arm.
		return b.EmitConstInst(true), nil
	case without.TupleStructPattern() != nil:
		text := without.TupleStructPattern().GetText()
		if strings.HasSuffix(strings.TrimSpace(strings.SplitN(text, "(", 2)[0]), "None") {
			return b.EmitBinOp(ssa.OpEq, subject, b.EmitConstInstNil()), nil
		}
		// TODO(ssa-rust): tuple/struct pattern fields are not destructured or bound.
		return b.EmitConstInst(true), nil
	}
	return b.EmitConstInst(true), nil
}

func (b *singleFileBuilder) rustRangePatternCondition(subject ssa.Value, text string) ssa.Value {
	text = strings.TrimSpace(text)
	separator := "..="
	if !strings.Contains(text, separator) {
		separator = ".."
	}
	parts := strings.SplitN(text, separator, 2)
	if len(parts) != 2 {
		return b.EmitConstInst(true)
	}
	low, lowErr := strconv.ParseInt(trimRustIntSuffix(strings.TrimSpace(parts[0])), 0, 64)
	high, highErr := strconv.ParseInt(trimRustIntSuffix(strings.TrimSpace(parts[1])), 0, 64)
	if lowErr != nil || highErr != nil {
		return b.EmitConstInst(true)
	}
	lower := b.EmitBinOp(ssa.OpGtEq, subject, b.EmitConstInst(low))
	var upperOp ssa.BinaryOpcode = ssa.OpLtEq
	if separator == ".." {
		upperOp = ssa.OpLt
	}
	upper := b.EmitBinOp(upperOp, subject, b.EmitConstInst(high))
	return b.EmitBinOp(ssa.OpAnd, lower, upper)
}

func (b *singleFileBuilder) rustLiteralPatternValue(raw rustparser.ILiteralPatternContext) ssa.Value {
	literal, ok := raw.(*rustparser.LiteralPatternContext)
	if !ok || literal == nil {
		return nil
	}
	negative := literal.MINUS() != nil
	switch {
	case literal.KW_TRUE() != nil:
		return b.EmitConstInst(true)
	case literal.KW_FALSE() != nil:
		return b.EmitConstInst(false)
	case literal.INTEGER_LITERAL() != nil:
		text := trimRustIntSuffix(literal.INTEGER_LITERAL().GetText())
		if value, err := strconv.ParseInt(text, 0, 64); err == nil {
			if negative {
				value = -value
			}
			return b.EmitConstInst(value)
		}
	case literal.FLOAT_LITERAL() != nil:
		text := trimRustFloatSuffix(literal.FLOAT_LITERAL().GetText())
		if value, err := strconv.ParseFloat(text, 64); err == nil {
			if negative {
				value = -value
			}
			return b.EmitConstInst(value)
		}
	case literal.STRING_LITERAL() != nil:
		return b.EmitConstInst(unquoteRustString(literal.STRING_LITERAL().GetText()))
	case literal.RAW_STRING_LITERAL() != nil:
		return b.EmitConstInst(unquoteRustString(literal.RAW_STRING_LITERAL().GetText()))
	case literal.CHAR_LITERAL() != nil:
		return b.EmitConstInst(strings.Trim(literal.CHAR_LITERAL().GetText(), "'"))
	}
	return nil
}

func (b *singleFileBuilder) visitStructExpression(raw rustparser.IStructExpressionContext) ssa.Value {
	expression, ok := raw.(*rustparser.StructExpressionContext)
	if !ok || expression == nil {
		return b.EmitUndefined("struct")
	}
	container := b.EmitEmptyContainer()
	if unit := expression.StructExprUnit(); unit != nil {
		return container
	}
	if tuple, ok := expression.StructExprTuple().(*rustparser.StructExprTupleContext); ok && tuple != nil {
		for index, rawExpr := range tuple.AllExpression() {
			value := b.VisitExpression(rawExpr)
			if value == nil {
				value = b.EmitConstInstNil()
			}
			key := b.EmitConstInst(int64(index))
			b.AssignVariable(b.CreateMemberCallVariable(container, key), value)
		}
		return container
	}
	structExpr, ok := expression.StructExprStruct().(*rustparser.StructExprStructContext)
	if !ok || structExpr == nil || structExpr.StructExprFields() == nil {
		return container
	}
	fields, ok := structExpr.StructExprFields().(*rustparser.StructExprFieldsContext)
	if !ok || fields == nil {
		return container
	}
	for _, rawField := range fields.AllStructExprField() {
		field, ok := rawField.(*rustparser.StructExprFieldContext)
		if !ok || field == nil {
			continue
		}
		name := ""
		if field.Identifier() != nil {
			name = field.Identifier().GetText()
		} else if field.TupleIndex() != nil {
			name = field.TupleIndex().GetText()
		}
		if name == "" {
			continue
		}
		value := ssa.Value(nil)
		if field.COLON() != nil && field.Expression() != nil {
			value = b.VisitExpression(field.Expression())
		} else {
			value = b.ReadValue(name)
		}
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateMemberCallVariable(container, b.EmitConstInst(name)), value)
	}
	return container
}

func (b *singleFileBuilder) visitArrayExpression(ctx *rustparser.ArrayExpressionContext) ssa.Value {
	container := b.EmitEmptyContainer()
	if ctx == nil || ctx.ArrayElements() == nil {
		return container
	}
	elements, ok := ctx.ArrayElements().(*rustparser.ArrayElementsContext)
	if !ok || elements == nil {
		return container
	}
	for index, rawExpr := range elements.AllExpression() {
		value := b.VisitExpression(rawExpr)
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateMemberCallVariable(container, b.EmitConstInst(int64(index))), value)
	}
	return container
}

func (b *singleFileBuilder) visitTupleExpression(ctx *rustparser.TupleExpressionContext) ssa.Value {
	container := b.EmitEmptyContainer()
	if ctx == nil || ctx.TupleElements() == nil {
		return container
	}
	elements, ok := ctx.TupleElements().(*rustparser.TupleElementsContext)
	if !ok || elements == nil {
		return container
	}
	for index, rawExpr := range elements.AllExpression() {
		value := b.VisitExpression(rawExpr)
		if value == nil {
			value = b.EmitConstInstNil()
		}
		b.AssignVariable(b.CreateMemberCallVariable(container, b.EmitConstInst(int64(index))), value)
	}
	return container
}
