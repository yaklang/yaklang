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

	switch ret := raw.(type) {
	case *phpparser.CloneExpressionContext:
		// 浅拷贝
		// 如果类定义了 __clone，就执行 __clone
	case *phpparser.KeywordNewExpressionContext:
		return y.VisitNewExpr(ret.NewExpr())
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
		if ret.Constant() != nil {
			return y.VisitConstant(ret.Constant())
		} else if ret.String_() != nil {
			return y.VisitString_(ret.String_())
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
	case *phpparser.InstanceOfExpressionContext:
	case *phpparser.ComparisonExpressionContext:
	case *phpparser.BitwiseExpressionContext:
	case *phpparser.ConditionalExpressionContext:
	case *phpparser.NullCoalescingExpressionContext:
	case *phpparser.SpaceshipExpressionContext:
	case *phpparser.ArrayDestructExpressionContext:
	case *phpparser.AssignmentExpressionContext:
	case *phpparser.LogicalExpressionContext:
	default:
		_ = ret
	}

	return nil
}
