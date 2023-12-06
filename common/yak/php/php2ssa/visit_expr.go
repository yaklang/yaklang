package php2ssa

import (
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

	return nil
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
		y.VisitExpression(i.Expression())
	} else if i.YieldExpression() != nil {
		y.VisitYieldExpression(i.YieldExpression())
	}

	return nil
}

func (y *builder) VisitExpression(raw phpparser.IExpressionContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ExpressionContext)
	if i == nil {
		return nil
	}

	switch i.(type) {
	case *phpparser.CloneExpressionContext:
	case *phpparser.KeywordNewExpressionContext:
	case *phpparser.IndexerExpressionContext:
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
	case *phpparser.ChainExpressionContext:
	case *phpparser.ScalarExpressionContext: // constant / string / label
	case *phpparser.BackQuoteStringExpressionContext:
	case *phpparser.ParenthesisExpressionContext:
	case *phpparser.SpecialWordExpressionContext:
	case *phpparser.LambdaFunctionExpressionContext:
	case *phpparser.MatchExpressionContext:
	case *phpparser.ArithmeticExpressionContext:
	case *phpparser.InstanceOfExpressionContext:
	case *phpparser.ComparisonExpressionContext:
	case *phpparser.BitwiseExpressionContext:
	case *phpparser.ConditionalExpressionContext:
	case *phpparser.NullCoalescingExpressionContext:
	case *phpparser.SpaceshipExpressionContext:
	case *phpparser.ArrayDestructExpressionContext:
	case *phpparser.AssignmentExpressionContext:
	case *phpparser.LogicalExpressionContext:
	}

	return nil
}
