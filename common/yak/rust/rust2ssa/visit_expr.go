package rust2ssa

import (
	"strconv"

	"github.com/yaklang/antlr/v4"
	"strings"

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
		b.visitExpressionWithBlock(expr.ExpressionWithBlock())
		return nil
	case *rustparser.MacroInvocationAsExpressionContext:
		return b.visitMacro(expr.MacroInvocation())
	case *rustparser.StructExpression_Context:
		return b.EmitUndefined(raw.GetText())
	default:
		return b.EmitUndefined(raw.GetText())
	}
}

func (b *singleFileBuilder) visitExpressionWithBlock(raw rustparser.IExpressionWithBlockContext) {
	ctx, ok := raw.(*rustparser.ExpressionWithBlockContext)
	if !ok || ctx == nil {
		return
	}
	switch {
	case ctx.BlockExpression() != nil:
		b.visitBlockExpression(ctx.BlockExpression(), false)
	case ctx.IfExpression() != nil:
		if ifExpr, ok := ctx.IfExpression().(*rustparser.IfExpressionContext); ok {
			b.visitIf(ifExpr)
		}
	case ctx.LoopExpression() != nil:
		b.visitLoop(ctx.LoopExpression())
	}
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
	if method == nil {
		method = b.EmitUndefined(name)
	}
	args := b.visitCallParams(ctx.CallParams())
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
	left := b.VisitExpression(ctx.Expression(0))
	right := b.VisitExpression(ctx.Expression(1))
	if left == nil {
		left = b.EmitConstInstNil()
	}
	if right == nil {
		right = b.EmitConstInstNil()
	}
	operator := "&&"
	if ctx.OROR() != nil {
		operator = "||"
	}
	return b.EmitBinOp(rustBinaryOpcode(operator), left, right)
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
