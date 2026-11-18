package kotlin2ssa

import (
	"strconv"
	"strings"

	kotlinparser "github.com/yaklang/yaklang/common/yak/kotlin/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func kotlinBinaryOpcode(op string) ssa.BinaryOpcode {
	switch op {
	case "+", "++":
		return ssa.OpAdd
	case "-", "--":
		return ssa.OpSub
	case "*":
		return ssa.OpMul
	case "/":
		return ssa.OpDiv
	case "%":
		return ssa.OpMod
	case "&&":
		return ssa.OpAnd
	case "||":
		return ssa.OpOr
	case "==", "===":
		return ssa.OpEq
	case "!=", "!==":
		return ssa.OpNotEq
	case "<":
		return ssa.OpLt
	case ">":
		return ssa.OpGt
	case "<=":
		return ssa.OpLtEq
	case ">=":
		return ssa.OpGtEq
	case "in":
		return ssa.OpIn
	default:
		return ssa.OpAdd
	}
}

func (b *singleFileBuilder) VisitExpression(raw kotlinparser.IExpressionContext) ssa.Value {
	expression, ok := raw.(*kotlinparser.ExpressionContext)
	if !ok || expression == nil {
		return b.EmitConstInstNil()
	}
	if expression.Disjunction() != nil {
		return b.visitDisjunction(expression.Disjunction())
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitDisjunction(raw kotlinparser.IDisjunctionContext) ssa.Value {
	disjunction, ok := raw.(*kotlinparser.DisjunctionContext)
	if !ok || disjunction == nil || len(disjunction.AllConjunction()) == 0 {
		return b.EmitConstInstNil()
	}
	left := b.visitConjunction(disjunction.Conjunction(0))
	for i := 1; i < len(disjunction.AllConjunction()); i++ {
		right := b.visitConjunction(disjunction.Conjunction(i))
		left = b.EmitBinOp(ssa.OpOr, left, right)
	}
	return left
}

func (b *singleFileBuilder) visitConjunction(raw kotlinparser.IConjunctionContext) ssa.Value {
	conjunction, ok := raw.(*kotlinparser.ConjunctionContext)
	if !ok || conjunction == nil || len(conjunction.AllEquality()) == 0 {
		return b.EmitConstInstNil()
	}
	left := b.visitEquality(conjunction.Equality(0))
	for i := 1; i < len(conjunction.AllEquality()); i++ {
		right := b.visitEquality(conjunction.Equality(i))
		left = b.EmitBinOp(ssa.OpAnd, left, right)
	}
	return left
}

func (b *singleFileBuilder) visitEquality(raw kotlinparser.IEqualityContext) ssa.Value {
	equality, ok := raw.(*kotlinparser.EqualityContext)
	if !ok || equality == nil || len(equality.AllComparison()) == 0 {
		return b.EmitConstInstNil()
	}
	left := b.visitComparison(equality.Comparison(0))
	operators := equality.AllEqualityOperator()
	for i := 1; i < len(equality.AllComparison()); i++ {
		right := b.visitComparison(equality.Comparison(i))
		op := "=="
		if i-1 < len(operators) {
			op = operators[i-1].GetText()
		}
		left = b.EmitBinOp(kotlinBinaryOpcode(op), left, right)
	}
	return left
}

func (b *singleFileBuilder) visitComparison(raw kotlinparser.IComparisonContext) ssa.Value {
	comparison, ok := raw.(*kotlinparser.ComparisonContext)
	if !ok || comparison == nil || len(comparison.AllInfixOperation()) == 0 {
		return b.EmitConstInstNil()
	}
	left := b.visitInfixOperation(comparison.InfixOperation(0))
	if len(comparison.AllInfixOperation()) > 1 && comparison.ComparisonOperator() != nil {
		right := b.visitInfixOperation(comparison.InfixOperation(1))
		left = b.EmitBinOp(kotlinBinaryOpcode(comparison.ComparisonOperator().GetText()), left, right)
	}
	return left
}

func (b *singleFileBuilder) visitInfixOperation(raw kotlinparser.IInfixOperationContext) ssa.Value {
	infix, ok := raw.(*kotlinparser.InfixOperationContext)
	if !ok || infix == nil || len(infix.AllElvisExpression()) == 0 {
		return b.EmitConstInstNil()
	}
	left := b.visitElvisExpression(infix.ElvisExpression(0))
	var op ssa.BinaryOpcode = ssa.OpEq
	if len(infix.AllInOperator()) > 0 {
		op = ssa.OpIn
		if strings.Contains(infix.InOperator(0).GetText(), "!") {
			op = ssa.OpNotEq
		}
	}
	for i := 1; i < len(infix.AllElvisExpression()); i++ {
		right := b.visitElvisExpression(infix.ElvisExpression(i))
		left = b.EmitBinOp(op, left, right)
	}
	return left
}

func (b *singleFileBuilder) visitElvisExpression(raw kotlinparser.IElvisExpressionContext) ssa.Value {
	elvis, ok := raw.(*kotlinparser.ElvisExpressionContext)
	if !ok || elvis == nil || len(elvis.AllInfixFunctionCall()) == 0 {
		return b.EmitConstInstNil()
	}
	left := b.visitInfixFunctionCall(elvis.InfixFunctionCall(0))
	for i := 1; i < len(elvis.AllInfixFunctionCall()); i++ {
		right := b.visitInfixFunctionCall(elvis.InfixFunctionCall(i))
		left = b.EmitBinOp(ssa.OpOr, left, right)
	}
	return left
}

func (b *singleFileBuilder) visitInfixFunctionCall(raw kotlinparser.IInfixFunctionCallContext) ssa.Value {
	infix, ok := raw.(*kotlinparser.InfixFunctionCallContext)
	if !ok || infix == nil || len(infix.AllRangeExpression()) == 0 {
		return b.EmitConstInstNil()
	}
	left := b.visitRangeExpression(infix.AllRangeExpression()[0])
	names := infix.AllSimpleIdentifier()
	for i := 1; i < len(infix.AllRangeExpression()); i++ {
		right := b.visitRangeExpression(infix.AllRangeExpression()[i])
		name := "infix"
		if i-1 < len(names) {
			name = names[i-1].GetText()
		}
		left = b.EmitCall(b.NewCall(b.ReadValue(name), []ssa.Value{left, right}))
	}
	return left
}

func (b *singleFileBuilder) visitRangeExpression(raw kotlinparser.IRangeExpressionContext) ssa.Value {
	rangeExpr, ok := raw.(*kotlinparser.RangeExpressionContext)
	if !ok || rangeExpr == nil || len(rangeExpr.AllAdditiveExpression()) == 0 {
		return b.EmitConstInstNil()
	}
	left := b.visitAdditiveExpression(rangeExpr.AdditiveExpression(0))
	for i := 1; i < len(rangeExpr.AllAdditiveExpression()); i++ {
		right := b.visitAdditiveExpression(rangeExpr.AdditiveExpression(i))
		left = b.EmitBinOp(ssa.OpAdd, left, right)
	}
	return left
}

func (b *singleFileBuilder) visitAdditiveExpression(raw kotlinparser.IAdditiveExpressionContext) ssa.Value {
	additive, ok := raw.(*kotlinparser.AdditiveExpressionContext)
	if !ok || additive == nil || len(additive.AllMultiplicativeExpression()) == 0 {
		return b.EmitConstInstNil()
	}
	if len(additive.AllMultiplicativeExpression()) == 1 {
		return b.visitMultiplicativeExpression(additive.MultiplicativeExpression(0))
	}
	left := b.visitMultiplicativeExpression(additive.MultiplicativeExpression(0))
	operators := additive.AllAdditiveOperator()
	for i := 1; i < len(additive.AllMultiplicativeExpression()); i++ {
		right := b.visitMultiplicativeExpression(additive.MultiplicativeExpression(i))
		op := "+"
		if i-1 < len(operators) {
			op = operators[i-1].GetText()
		}
		left = b.EmitBinOp(kotlinBinaryOpcode(op), left, right)
	}
	return left
}

func (b *singleFileBuilder) visitMultiplicativeExpression(raw kotlinparser.IMultiplicativeExpressionContext) ssa.Value {
	multiplicative, ok := raw.(*kotlinparser.MultiplicativeExpressionContext)
	if !ok || multiplicative == nil || len(multiplicative.AllAsExpression()) == 0 {
		return b.EmitConstInstNil()
	}
	if len(multiplicative.AllAsExpression()) == 1 {
		return b.visitAsExpression(multiplicative.AsExpression(0))
	}
	left := b.visitAsExpression(multiplicative.AsExpression(0))
	operators := multiplicative.AllMultiplicativeOperator()
	for i := 1; i < len(multiplicative.AllAsExpression()); i++ {
		right := b.visitAsExpression(multiplicative.AsExpression(i))
		op := "*"
		if i-1 < len(operators) {
			op = operators[i-1].GetText()
		}
		left = b.EmitBinOp(kotlinBinaryOpcode(op), left, right)
	}
	return left
}

func (b *singleFileBuilder) visitAsExpression(raw kotlinparser.IAsExpressionContext) ssa.Value {
	asExpr, ok := raw.(*kotlinparser.AsExpressionContext)
	if !ok || asExpr == nil || asExpr.PrefixUnaryExpression() == nil {
		return b.EmitConstInstNil()
	}
	return b.visitPrefixUnaryExpression(asExpr.PrefixUnaryExpression())
}

func (b *singleFileBuilder) visitPrefixUnaryExpression(raw kotlinparser.IPrefixUnaryExpressionContext) ssa.Value {
	prefix, ok := raw.(*kotlinparser.PrefixUnaryExpressionContext)
	if !ok || prefix == nil || prefix.PostfixUnaryExpression() == nil {
		return b.EmitConstInstNil()
	}
	value := b.visitPostfixUnaryExpression(prefix.PostfixUnaryExpression())
	prefixes := prefix.AllUnaryPrefix()
	for i := len(prefixes) - 1; i >= 0; i-- {
		unary, ok := prefixes[i].(*kotlinparser.UnaryPrefixContext)
		if !ok || unary == nil || unary.PrefixUnaryOperator() == nil {
			continue
		}
		operator, ok := unary.PrefixUnaryOperator().(*kotlinparser.PrefixUnaryOperatorContext)
		if !ok || operator == nil {
			continue
		}
		switch {
		case operator.SUB() != nil:
			value = b.EmitUnOp(ssa.OpNeg, value)
		case operator.ADD() != nil:
			value = b.EmitUnOp(ssa.OpPlus, value)
		case operator.Excl() != nil:
			value = b.EmitUnOp(ssa.OpNot, value)
		case operator.INCR() != nil:
			value = b.EmitBinOp(ssa.OpAdd, value, b.EmitConstInst(1))
		case operator.DECR() != nil:
			value = b.EmitBinOp(ssa.OpSub, value, b.EmitConstInst(1))
		}
	}
	return value
}

func (b *singleFileBuilder) visitPostfixUnaryExpression(raw kotlinparser.IPostfixUnaryExpressionContext) ssa.Value {
	postfix, ok := raw.(*kotlinparser.PostfixUnaryExpressionContext)
	if !ok || postfix == nil || postfix.PrimaryExpression() == nil {
		return b.EmitConstInstNil()
	}
	value := b.visitPrimaryExpression(postfix.PrimaryExpression())
	for _, rawSuffix := range postfix.AllPostfixUnarySuffix() {
		suffix, ok := rawSuffix.(*kotlinparser.PostfixUnarySuffixContext)
		if !ok || suffix == nil {
			continue
		}
		switch {
		case suffix.CallSuffix() != nil:
			args := b.visitCallSuffixArguments(suffix.CallSuffix())
			value = b.EmitCall(b.NewCall(value, args))
		case suffix.IndexingSuffix() != nil:
			if index, ok := suffix.IndexingSuffix().(*kotlinparser.IndexingSuffixContext); ok && index != nil && len(index.AllExpression()) > 0 {
				key := b.VisitExpression(index.Expression(0))
				value = b.ReadMemberCallValue(value, key)
			}
		case suffix.NavigationSuffix() != nil:
			if nav, ok := suffix.NavigationSuffix().(*kotlinparser.NavigationSuffixContext); ok && nav != nil && nav.SimpleIdentifier() != nil {
				key := b.EmitConstInst(nav.SimpleIdentifier().GetText())
				value = b.ReadMemberCallValue(value, key)
			}
		case suffix.PostfixUnaryOperator() != nil:
			operator := suffix.PostfixUnaryOperator().GetText()
			if operator == "++" {
				value = b.EmitBinOp(ssa.OpAdd, value, b.EmitConstInst(1))
			} else if operator == "--" {
				value = b.EmitBinOp(ssa.OpSub, value, b.EmitConstInst(1))
			}
		}
	}
	return value
}

func (b *singleFileBuilder) visitCallSuffixArguments(raw kotlinparser.ICallSuffixContext) []ssa.Value {
	call, ok := raw.(*kotlinparser.CallSuffixContext)
	if !ok || call == nil || call.ValueArguments() == nil {
		return nil
	}
	args, ok := call.ValueArguments().(*kotlinparser.ValueArgumentsContext)
	if !ok || args == nil {
		return nil
	}
	result := make([]ssa.Value, 0, len(args.AllValueArgument()))
	for _, rawArg := range args.AllValueArgument() {
		arg, ok := rawArg.(*kotlinparser.ValueArgumentContext)
		if !ok || arg == nil || arg.Expression() == nil {
			continue
		}
		result = append(result, b.VisitExpression(arg.Expression()))
	}
	return result
}

func (b *singleFileBuilder) visitPrimaryExpression(raw kotlinparser.IPrimaryExpressionContext) ssa.Value {
	primary, ok := raw.(*kotlinparser.PrimaryExpressionContext)
	if !ok || primary == nil {
		return b.EmitConstInstNil()
	}
	switch {
	case primary.ParenthesizedExpression() != nil:
		if paren, ok := primary.ParenthesizedExpression().(*kotlinparser.ParenthesizedExpressionContext); ok && paren != nil && paren.Expression() != nil {
			return b.VisitExpression(paren.Expression())
		}
	case primary.LiteralConstant() != nil:
		return b.visitLiteralConstant(primary.LiteralConstant())
	case primary.StringLiteral() != nil:
		return b.EmitConstInst(strings.Trim(primary.StringLiteral().GetText(), "\""))
	case primary.SimpleIdentifier() != nil:
		return b.ReadValue(primary.SimpleIdentifier().GetText())
	case primary.CollectionLiteral() != nil:
		return b.visitCollectionLiteral(primary.CollectionLiteral())
	case primary.IfExpression() != nil:
		b.visitIfExpression(primary.IfExpression())
		return b.EmitConstInstNil()
	case primary.JumpExpression() != nil:
		b.visitJumpExpression(primary.JumpExpression())
		return b.EmitConstInstNil()
	case primary.WhenExpression() != nil:
		return b.visitWhenExpression(primary.WhenExpression())
	case primary.ThisExpression() != nil:
		return b.ReadValue("this")
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitLiteralConstant(raw kotlinparser.ILiteralConstantContext) ssa.Value {
	literal, ok := raw.(*kotlinparser.LiteralConstantContext)
	if !ok || literal == nil {
		return b.EmitConstInstNil()
	}
	switch {
	case literal.BooleanLiteral() != nil:
		return b.EmitConstInst(literal.BooleanLiteral().GetText() == "true")
	case literal.IntegerLiteral() != nil:
		text := strings.ReplaceAll(literal.IntegerLiteral().GetText(), "_", "")
		if value, err := strconv.ParseInt(text, 0, 64); err == nil {
			return b.EmitConstInst(int64(value))
		}
	case literal.HexLiteral() != nil:
		text := strings.ReplaceAll(literal.HexLiteral().GetText(), "_", "")
		if value, err := strconv.ParseInt(text, 0, 64); err == nil {
			return b.EmitConstInst(int64(value))
		}
	case literal.BinLiteral() != nil:
		text := strings.ReplaceAll(strings.TrimPrefix(literal.BinLiteral().GetText(), "0b"), "_", "")
		if value, err := strconv.ParseInt(text, 2, 64); err == nil {
			return b.EmitConstInst(int64(value))
		}
	case literal.LongLiteral() != nil:
		text := strings.TrimSuffix(strings.ReplaceAll(literal.LongLiteral().GetText(), "_", ""), "L")
		if value, err := strconv.ParseInt(text, 0, 64); err == nil {
			return b.EmitConstInst(int64(value))
		}
	case literal.RealLiteral() != nil:
		text := strings.TrimSuffix(strings.ReplaceAll(literal.RealLiteral().GetText(), "_", ""), "f")
		if value, err := strconv.ParseFloat(text, 64); err == nil {
			return b.EmitConstInst(value)
		}
	case literal.CharacterLiteral() != nil:
		return b.EmitConstInst(strings.Trim(literal.CharacterLiteral().GetText(), "'"))
	case literal.NullLiteral() != nil:
		return b.EmitConstInstNil()
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitCollectionLiteral(raw kotlinparser.ICollectionLiteralContext) ssa.Value {
	collection, ok := raw.(*kotlinparser.CollectionLiteralContext)
	if !ok || collection == nil {
		return b.EmitEmptyContainer()
	}
	container := b.EmitEmptyContainer()
	for i, rawExpression := range collection.AllExpression() {
		value := b.VisitExpression(rawExpression)
		key := b.EmitConstInst(int64(i))
		b.AssignVariable(b.CreateMemberCallVariable(container, key), value)
	}
	return container
}

func (b *singleFileBuilder) visitWhenExpression(raw kotlinparser.IWhenExpressionContext) ssa.Value {
	when, ok := raw.(*kotlinparser.WhenExpressionContext)
	if !ok || when == nil {
		return b.EmitConstInstNil()
	}
	var subject ssa.Value
	if when.Expression() != nil {
		subject = b.VisitExpression(when.Expression())
	}
	ifBuilder := b.CreateIfBuilder()
	for _, rawEntry := range when.AllWhenEntry() {
		entry, ok := rawEntry.(*kotlinparser.WhenEntryContext)
		if !ok || entry == nil {
			continue
		}
		if entry.ELSE() != nil {
			ifBuilder.SetElse(func() {
				b.visitControlStructureBody(entry.ControlStructureBody())
			})
			continue
		}
		conditions := make([]kotlinparser.IWhenConditionContext, 0, len(entry.AllWhenCondition()))
		for i := 0; i < len(entry.AllWhenCondition()); i++ {
			conditions = append(conditions, entry.WhenCondition(i))
		}
		ifBuilder.AppendItem(func() ssa.Value {
			var result ssa.Value
			for _, rawCondition := range conditions {
				condition, ok := rawCondition.(*kotlinparser.WhenConditionContext)
				if !ok || condition == nil {
					continue
				}
				var current ssa.Value
				if condition.TypeTest() != nil {
					current = b.EmitConstInst(true)
				} else if condition.Expression() != nil {
					current = b.EmitBinOp(ssa.OpEq, subject, b.VisitExpression(condition.Expression()))
				} else {
					current = b.EmitConstInst(true)
				}
				if result == nil {
					result = current
				} else {
					result = b.EmitBinOp(ssa.OpOr, result, current)
				}
			}
			if result == nil {
				result = b.EmitConstInst(true)
			}
			return result
		}, func() {
			b.visitControlStructureBody(entry.ControlStructureBody())
		})
	}
	ifBuilder.Build()
	return b.EmitConstInstNil()
}
