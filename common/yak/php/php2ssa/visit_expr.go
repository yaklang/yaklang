package php2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitExpressionStatement(raw phpparser.IExpressionStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ExpressionStatementContext)
	if i == nil {
		return nil
	}

	va := y.VisitExpression(i.Expression())
	return va
}

func (y *builder) VisitParentheses(raw phpparser.IParenthesesContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ParenthesesContext)
	if i == nil {
		return nil
	}

	if i.Expression() != nil {
		return y.VisitExpression(i.Expression())
	} else if i.YieldExpression() != nil {
		y.VisitYieldExpression(i.YieldExpression())
	}

	return nil
}

func (y *builder) VisitExpression(raw phpparser.IExpressionContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	if raw.GetText() == "" {
		return nil
	}

	switch ret := raw.(type) {
	case *phpparser.CloneExpressionContext:
		// 浅拷贝，一个对象
		// 如果类定义了 __clone，就执行 __clone
		target := y.VisitExpression(ret.Expression())
		checkCloneBuildin := y.ir.BuildIf()
		checkCloneBuildin.BuildCondition(func() ssa.Value {
			return y.ir.EmitBinOp(
				ssa.OpNotEq,
				y.ir.EmitField(target, y.ir.EmitConstInst("__clone")),
				y.ir.EmitConstInstNil(),
			)
		})

		checkCloneBuildin.BuildTrue(func() {
			// have __clone
			calling := y.ir.NewCall(
				y.ir.EmitField(target, y.ir.EmitConstInst("__clone")),
				nil,
			)
			y.ir.EmitCall(calling)
		})

		//
		return nil
	case *phpparser.KeywordNewExpressionContext:
		return y.VisitNewExpr(ret.NewExpr())
	case *phpparser.IndexerExpressionContext:
		v1 := y.VisitStringConstant(ret.StringConstant())
		indexKey := y.VisitExpression(ret.Expression())
		_ = v1
		_ = indexKey
	case *phpparser.CastExpressionContext:
	case *phpparser.UnaryOperatorExpressionContext:
		/*
			| ('~' | '@') expression                                      # UnaryOperatorExpression
			| ('!' | '+' | '-') expression                                # UnaryOperatorExpression
		*/
	case *phpparser.PrefixIncDecExpressionContext:
	case *phpparser.PostfixIncDecExpressionContext:
	case *phpparser.PrintExpressionContext:
	case *phpparser.ArrayCreationExpressionContext:
		// arrayCreation
		v := y.VisitArrayCreation(ret.ArrayCreation())
		_ = v

	case *phpparser.ChainExpressionContext:
	case *phpparser.ScalarExpressionContext: // constant / string / label
		if i := ret.Constant(); i != nil {
			return y.VisitConstant(i)
		} else if i := ret.String_(); i != nil {
			return y.VisitString_(i)
		} else if ret.Label() != nil {
			// break

		} else {
			log.Warnf("PHP Scalar Expr Failed: %s", ret.GetText())
		}
	case *phpparser.BackQuoteStringExpressionContext:
	case *phpparser.ParenthesisExpressionContext:
	case *phpparser.SpecialWordExpressionContext:
	case *phpparser.LambdaFunctionExpressionContext:
	case *phpparser.MatchExpressionContext:
	case *phpparser.ArithmeticExpressionContext:
		op1 := y.VisitExpression(ret.Expression(0))
		op2 := y.VisitExpression(ret.Expression(1))
		var o ssa.BinaryOpcode
		opStr := ret.GetOp().GetText()
		switch opStr {
		case "**":
			o = ssa.OpPow
		case "+":
			o = ssa.OpAdd
		case "-":
			o = ssa.OpSub
		case "*":
			o = ssa.OpMul
		case "/":
			o = ssa.OpDiv
		case "%":
			o = ssa.OpMod
		case ".":
			return y.ir.EmitFieldMust(op1, op2)
		default:

			return nil
		}
		return y.ir.EmitBinOp(o, op1, op2)
	case *phpparser.InstanceOfExpressionContext:
	case *phpparser.ComparisonExpressionContext:
	case *phpparser.BitwiseExpressionContext:
	case *phpparser.ConditionalExpressionContext:
	case *phpparser.NullCoalescingExpressionContext:
	case *phpparser.SpaceshipExpressionContext:
	case *phpparser.ArrayDestructExpressionContext:
	case *phpparser.AssignmentExpressionContext:
		if ret.AssignmentOperator() != nil {
			// assignable assignmentOperator attributes? expression        # AssignmentExpression

			// left value: chain array creation
			leftValues := y.VisitAssignable(ret.Assignable())
			_ = leftValues

			operator := ret.AssignmentOperator()
			_ = operator

			var annotation any
			if ret.Attributes() != nil {
				annotation = y.VisitAttributes(ret.Attributes())
				_ = annotation
			}

			rightValue := y.VisitExpression(ret.Expression())
			_ = rightValue
		} else if ret.Ampersand() != nil {
			// assignable Eq attributes? '&' (chain | newExpr)
			leftValues := y.VisitAssignable(ret.Assignable())
			if ret.Attributes() != nil {
				y.VisitAttributes(ret.Attributes())
			}

			// right val
			if i := ret.Chain(); i != nil {
				y.VisitChain(i)
			} else if i := ret.NewExpr(); i != nil {
				y.VisitNewExpr(i)
			}
			_ = leftValues
		}

	case *phpparser.LogicalExpressionContext:
	default:
		log.Errorf("unhandled expression: %v(%T)", ret.GetText(), ret)
		log.Errorf("-------------unhandled expression: %v(%T)", ret.GetText(), ret)
		_ = ret
	}

	return nil
}

func (y *builder) VisitAssignable(raw phpparser.IAssignableContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.AssignableContext)
	if i == nil {
		return nil
	}

	if i.Chain() != nil {
		return y.VisitChain(i.Chain())
	} else if i.ArrayCreation() != nil {
		return y.VisitArrayCreation(i.ArrayCreation())
	} else {
		return nil
	}

	return nil
}

func (y *builder) VisitChain(raw phpparser.IChainContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ChainContext)
	if i == nil {
		return nil
	}

	y.VisitChainOrigin(i.ChainOrigin())

	for _, m := range i.AllMemberAccess() {
		y.VisitMemberAccess(m)
	}

	return nil
}

func (y *builder) VisitMemberAccess(raw phpparser.IMemberAccessContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.MemberAccessContext)
	if i == nil {
		return nil
	}

	y.VisitKeyedFieldName(i.KeyedFieldName())
	if i.ActualArguments() != nil {
		y.VisitActualArguments(i.ActualArguments())
	}

	return nil
}

func (y *builder) VisitActualArguments(raw phpparser.IActualArgumentsContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ActualArgumentsContext)
	if i == nil {
		return nil
	}

	// PHP8 annotation
	for _, a := range i.AllArguments() {
		y.VisitArguments(a)
	}

	for _, a := range i.AllSquareCurlyExpression() {
		y.VisitSquareCurlyExpression(a)
	}

	return nil
}

func (y *builder) VisitKeyedFieldName(raw phpparser.IKeyedFieldNameContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.KeyedFieldNameContext)
	if i == nil {
		return nil
	}

	if i.KeyedSimpleFieldName() != nil {
		y.VisitKeyedSimpleFieldName(i.KeyedSimpleFieldName())
	} else if i.KeyedVariable() != nil {
		y.VisitKeyedVariable(i.KeyedVariable())
	}

	return nil
}

func (y *builder) VisitKeyedVariable(raw phpparser.IKeyedVariableContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.KeyedVariableContext)
	if i == nil {
		return nil
	}

	dollarCount := 0
	if i.VarName() != nil {
		dollarCount = len(i.AllDollar())
	} else {
		dollarCount = len(i.AllDollar()) - 1
	}
	_ = dollarCount

	v := y.VisitExpression(i.Expression())
	_ = v
	var sv []any
	for _, a := range i.AllSquareCurlyExpression() {
		sv = append(sv, y.VisitSquareCurlyExpression(a))
	}

	return nil
}

func (y *builder) VisitKeyedSimpleFieldName(raw phpparser.IKeyedSimpleFieldNameContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.KeyedSimpleFieldNameContext)
	if i == nil {
		return nil
	}

	if i.Identifier() != nil {
		v := y.VisitIdentifier(i.Identifier())
		_ = v
	} else if i.Expression() != nil {
		v := y.VisitExpression(i.Expression())
		_ = v
	}

	for _, sce := range i.AllSquareCurlyExpression() {
		y.VisitSquareCurlyExpression(sce)
	}

	return nil
}

func (y *builder) VisitSquareCurlyExpression(raw phpparser.ISquareCurlyExpressionContext) ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.SquareCurlyExpressionContext)
	if i == nil {
		return nil
	}

	if i.OpenSquareBracket() != nil {
		if i.Expression() != nil {
			v := y.VisitExpression(i.Expression())
			_ = v
		}
	} else {
		return y.VisitExpression(i.Expression())
	}

	return nil
}

func (y *builder) VisitChainOrigin(raw phpparser.IChainOriginContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ChainOriginContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitArrayCreation(raw phpparser.IArrayCreationContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ArrayCreationContext)
	if i == nil {
		return nil
	}

	mapIns := y.ir.EmitMakeBuildWithType(
		ssa.NewMapType(
			ssa.GetTypeByStr("any"),
			ssa.GetTypeByStr("any"),
		),
		y.ir.EmitConstInst(0),
		y.ir.EmitConstInst(0),
	)
	for _, kv := range y.VisitArrayItemList(i.ArrayItemList()) {
		_ = kv
	}
	_ = mapIns
	if i.Expression() != nil {
		y.VisitExpression(i.Expression())
	}
	return nil
}

func (y *builder) VisitArrayItemList(raw phpparser.IArrayItemListContext) [][2]ssa.Value {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ArrayItemListContext)
	if i == nil {
		return nil
	}

	countIndex := 0
	var results [][2]ssa.Value
	for _, a := range i.AllArrayItem() {

		k, v := y.VisitArrayItem(a)
		if k != nil {
			k = y.ir.EmitConstInst(countIndex)
			countIndex++
		}
		kv := [2]ssa.Value{k, v}
		results = append(results, kv)
	}
	return results
}

func (y *builder) VisitArrayItem(raw phpparser.IArrayItemContext) (ssa.Value, ssa.Value) {
	if y == nil || raw == nil {
		return nil, nil
	}

	i, _ := raw.(*phpparser.ArrayItemContext)
	if i == nil {
		return nil, nil
	}

	if i.Chain() != nil {
		// (expression '=>')? '&' chain
		var v ssa.Value
		if i.Expression(0) != nil {
			v = y.VisitExpression(i.Expression(0))
		}
		return v, y.VisitChain(i.Chain())
	} else {
		// expression ('=>' expression)?
		k := y.VisitExpression(i.Expression(0))
		var v ssa.Value
		if i.Expression(1) != nil {
			v = y.VisitExpression(i.Expression(1))
		}
		return k, v
	}
}

func (y *builder) VisitAttributes(raw phpparser.IAttributesContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.AttributesContext)
	if i == nil {
		return nil
	}

	for _, g := range i.AllAttributeGroup() {
		y.VisitAttributeGroup(g)
	}

	return nil
}

func (y *builder) VisitAttributeGroup(raw phpparser.IAttributeGroupContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.AttributeGroupContext)
	if i == nil {
		return nil
	}

	y.VisitIdentifier(i.Identifier())

	for _, a := range i.AllAttribute() {
		y.VisitAttribute(a)
	}

	return nil
}

func (y *builder) VisitAttribute(raw phpparser.IAttributeContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.AttributeContext)
	if i == nil {
		return nil
	}

	y.VisitQualifiedNamespaceName(i.QualifiedNamespaceName())
	if i.Arguments() != nil {
		y.VisitArguments(i.Arguments())
	}

	return nil
}

func (y *builder) VisitStringConstant(raw phpparser.IStringConstantContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.StringConstantContext)
	if i == nil {
		return nil
	}

	return y.ir.ReadVariable(i.GetText(), false)
}
