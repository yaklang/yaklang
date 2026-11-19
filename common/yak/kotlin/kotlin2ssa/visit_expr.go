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
	result := func() ssa.Value { return b.visitConjunction(disjunction.Conjunction(0)) }
	for i := 1; i < len(disjunction.AllConjunction()); i++ {
		next := disjunction.Conjunction(i)
		previous := result
		result = func() ssa.Value {
			return b.BuildShortCircuit(previous, func() ssa.Value { return b.visitConjunction(next) }, false)
		}
	}
	return result()
}

func (b *singleFileBuilder) visitConjunction(raw kotlinparser.IConjunctionContext) ssa.Value {
	conjunction, ok := raw.(*kotlinparser.ConjunctionContext)
	if !ok || conjunction == nil || len(conjunction.AllEquality()) == 0 {
		return b.EmitConstInstNil()
	}
	result := func() ssa.Value { return b.visitEquality(conjunction.Equality(0)) }
	for i := 1; i < len(conjunction.AllEquality()); i++ {
		next := conjunction.Equality(i)
		previous := result
		result = func() ssa.Value {
			return b.BuildShortCircuit(previous, func() ssa.Value { return b.visitEquality(next) }, true)
		}
	}
	return result()
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
	value := b.visitPrefixUnaryExpression(asExpr.PrefixUnaryExpression())
	if asExpr.AsOperator() == nil {
		return value
	}
	typ := ssa.CreateAnyType()
	if asExpr.Type_() != nil {
		typ = kotlinTypeFromText(asExpr.Type_().GetText())
	}
	// TODO(ssa-kotlin): `as?` failure/null and smart casts are not modeled; both forms emit TypeCast.
	return b.EmitTypeCast(value, typ)
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
	isThis := strings.HasPrefix(postfix.GetText(), "this.")
	if primary, ok := postfix.PrimaryExpression().(*kotlinparser.PrimaryExpressionContext); ok && primary != nil {
		isThis = isThis || primary.ThisExpression() != nil
	}
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
				if isThis || (value != nil && value.GetName() == "this") {
					// Methods receive class fields as plain SSA variables.
					// Keep `this.x` on the same variable instead of building a
					// member on an undefined `this` object.
					if field := b.ReadValue(nav.SimpleIdentifier().GetText()); field != nil {
						value = field
						continue
					}
				}
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
		return b.visitIfExpression(primary.IfExpression())
	case primary.JumpExpression() != nil:
		b.visitJumpExpression(primary.JumpExpression())
		return b.EmitConstInstNil()
	case primary.WhenExpression() != nil:
		return b.visitWhenExpression(primary.WhenExpression())
	case primary.FunctionLiteral() != nil:
		return b.visitFunctionLiteral(primary.FunctionLiteral())
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
	case literal.UnsignedLiteral() != nil:
		text := strings.TrimRight(literal.UnsignedLiteral().GetText(), "uUlL")
		if value, err := strconv.ParseInt(strings.ReplaceAll(text, "_", ""), 0, 64); err == nil {
			return b.EmitConstInst(int64(value))
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

func (b *singleFileBuilder) visitWhenSubject(raw kotlinparser.IWhenSubjectContext) ssa.Value {
	subject, ok := raw.(*kotlinparser.WhenSubjectContext)
	if !ok || subject == nil || subject.Expression() == nil {
		return b.EmitConstInstNil()
	}
	return b.VisitExpression(subject.Expression())
}

func (b *singleFileBuilder) visitWhenExpression(raw kotlinparser.IWhenExpressionContext) ssa.Value {
	// TODO(ssa-kotlin): sealed/enum exhaustiveness and subject-less when conditions are not checked.
	when, ok := raw.(*kotlinparser.WhenExpressionContext)
	if !ok || when == nil {
		return b.EmitConstInstNil()
	}
	var subject ssa.Value
	if when.WhenSubject() != nil {
		subject = b.visitWhenSubject(when.WhenSubject())
	}
	cases := make([]ssa.MatchCase, 0, len(when.AllWhenEntry()))
	var otherwise func() ssa.Value
	for _, rawEntry := range when.AllWhenEntry() {
		entry, ok := rawEntry.(*kotlinparser.WhenEntryContext)
		if !ok || entry == nil {
			continue
		}
		if entry.ELSE() != nil {
			otherwise = func() ssa.Value {
				return b.visitControlStructureBodyValue(entry.ControlStructureBody())
			}
			continue
		}
		conditions := entry.AllWhenCondition()
		cases = append(cases, ssa.MatchCase{
			Condition: func() ssa.Value {
				var result ssa.Value
				for _, rawCondition := range conditions {
					condition, ok := rawCondition.(*kotlinparser.WhenConditionContext)
					if !ok || condition == nil {
						continue
					}
					value := b.visitWhenCondition(subject, condition)
					if value == nil {
						value = b.EmitConstInst(true)
					}
					if result == nil {
						result = value
					} else {
						result = b.EmitBinOp(ssa.OpOr, result, value)
					}
				}
				if result == nil {
					result = b.EmitConstInst(true)
				}
				return result
			},
			Body: func() ssa.Value {
				return b.visitControlStructureBodyValue(entry.ControlStructureBody())
			},
		})
	}
	return b.BuildMatchValue(cases, otherwise)
}

func (b *singleFileBuilder) visitWhenCondition(subject ssa.Value, condition *kotlinparser.WhenConditionContext) ssa.Value {
	switch {
	case condition.TypeTest() != nil:
		// TODO(ssa-kotlin): type tests use a synthetic `is` call; smart casts are not inferred.
		test, ok := condition.TypeTest().(*kotlinparser.TypeTestContext)
		if !ok || test == nil || test.Type_() == nil {
			return b.EmitConstInst(true)
		}
		value := b.emitKotlinIsType(subject, test.Type_().GetText())
		if test.IsOperator() != nil && strings.Contains(test.IsOperator().GetText(), "!") {
			return b.EmitUnOp(ssa.OpNot, value)
		}
		return value
	case condition.RangeTest() != nil:
		test, ok := condition.RangeTest().(*kotlinparser.RangeTestContext)
		if !ok || test == nil || test.Expression() == nil || subject == nil {
			return b.EmitConstInst(true)
		}
		rangeValue := b.VisitExpression(test.Expression())
		value := b.EmitBinOp(ssa.OpIn, subject, rangeValue)
		if test.InOperator() != nil && strings.Contains(test.InOperator().GetText(), "!") {
			return b.EmitUnOp(ssa.OpNot, value)
		}
		return value
	case condition.Expression() != nil:
		value := b.VisitExpression(condition.Expression())
		if subject == nil {
			return value
		}
		return b.EmitBinOp(ssa.OpEq, subject, value)
	}
	// TODO(ssa-kotlin): unsupported when-condition forms become a catch-all true branch.
	return b.EmitConstInst(true)
}

func (b *singleFileBuilder) emitKotlinIsType(value ssa.Value, typeName string) ssa.Value {
	if value == nil {
		value = b.EmitUndefined("value")
	}
	callee := b.ReadValue("is")
	if callee == nil {
		callee = b.EmitUndefined("is")
	}
	result := b.EmitCall(b.NewCall(callee, []ssa.Value{value, b.EmitConstInst(typeName)}))
	if result != nil {
		result.SetType(ssa.CreateBooleanType())
	}
	return result
}

func kotlinTypeFromText(text string) ssa.Type {
	text = strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(text), "?"))
	if index := strings.IndexByte(text, '<'); index >= 0 {
		text = text[:index]
	}
	switch text {
	case "Double", "Float":
		return ssa.CreateFloatType()
	case "Int", "Long", "Short", "Byte", "UInt", "ULong", "UShort", "UByte":
		return ssa.CreateNumberType()
	case "String", "Char", "CharSequence":
		return ssa.CreateStringType()
	case "Boolean":
		return ssa.CreateBooleanType()
	default:
		// TODO(ssa-kotlin): user-defined, sealed, and generic types collapse to any.
		return ssa.CreateAnyType()
	}
}

func (b *singleFileBuilder) visitFunctionLiteral(raw kotlinparser.IFunctionLiteralContext) ssa.Value {
	literal, ok := raw.(*kotlinparser.FunctionLiteralContext)
	if !ok || literal == nil {
		return b.EmitConstInstNil()
	}
	switch {
	case literal.LambdaLiteral() != nil:
		return b.visitLambdaLiteral(literal.LambdaLiteral())
	case literal.AnonymousFunction() != nil:
		return b.visitAnonymousFunction(literal.AnonymousFunction())
	}
	return b.EmitConstInstNil()
}

func (b *singleFileBuilder) visitLambdaLiteral(raw kotlinparser.ILambdaLiteralContext) ssa.Value {
	lambda, ok := raw.(*kotlinparser.LambdaLiteralContext)
	if !ok || lambda == nil {
		return b.EmitConstInstNil()
	}
	previous := b.FunctionBuilder
	fn := b.newClosureFunction(previous)
	b.FunctionBuilder = previous.PushFunction(fn)
	b.SupportClosure = true
	b.SetForceCapture(true)
	if params := lambda.LambdaParameters(); params != nil {
		if list, ok := params.(*kotlinparser.LambdaParametersContext); ok && list != nil {
			for _, rawParam := range list.AllLambdaParameter() {
				param, ok := rawParam.(*kotlinparser.LambdaParameterContext)
				if !ok || param == nil {
					continue
				}
				b.addLambdaParameter(param)
			}
		}
	}
	if value := b.visitStatementsValue(lambda.Statements()); value != nil {
		b.EmitReturn([]ssa.Value{value})
	}
	b.Finish()
	b.SetForceCapture(false)
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	return fn
}

func (b *singleFileBuilder) addLambdaParameter(param *kotlinparser.LambdaParameterContext) {
	if param == nil {
		return
	}
	if variable := param.VariableDeclaration(); variable != nil {
		if v, ok := variable.(*kotlinparser.VariableDeclarationContext); ok && v.SimpleIdentifier() != nil {
			b.NewParam(v.SimpleIdentifier().GetText())
		}
		return
	}
	if multi := param.MultiVariableDeclaration(); multi != nil {
		if m, ok := multi.(*kotlinparser.MultiVariableDeclarationContext); ok && m != nil {
			for _, rawVariable := range m.AllVariableDeclaration() {
				if v, ok := rawVariable.(*kotlinparser.VariableDeclarationContext); ok && v.SimpleIdentifier() != nil {
					b.NewParam(v.SimpleIdentifier().GetText())
				}
			}
		}
	}
}

func (b *singleFileBuilder) visitAnonymousFunction(raw kotlinparser.IAnonymousFunctionContext) ssa.Value {
	fnCtx, ok := raw.(*kotlinparser.AnonymousFunctionContext)
	if !ok || fnCtx == nil {
		return b.EmitConstInstNil()
	}
	previous := b.FunctionBuilder
	fn := b.newClosureFunction(previous)
	b.FunctionBuilder = previous.PushFunction(fn)
	b.SupportClosure = true
	b.SetForceCapture(true)
	if params := fnCtx.FunctionValueParameters(); params != nil {
		b.addFunctionValueParameters(params)
	}
	if body := fnCtx.FunctionBody(); body != nil {
		b.visitFunctionBody(body)
	}
	b.Finish()
	b.SetForceCapture(false)
	b.FunctionBuilder = b.PopFunction()
	b.FunctionBuilder = previous
	return fn
}

func (b *singleFileBuilder) newClosureFunction(previous *ssa.FunctionBuilder) *ssa.Function {
	if previous == nil {
		return nil
	}
	previousSupport := previous.SupportClosure
	previous.SupportClosure = true
	fn := previous.NewFunc("")
	previous.SupportClosure = previousSupport
	return fn
}
