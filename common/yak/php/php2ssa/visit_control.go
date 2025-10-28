//go:build !no_language
// +build !no_language

package php2ssa

import (
	"github.com/yaklang/yaklang/common/log"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitBreakStatement(raw phpparser.IBreakStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.BreakStatementContext)
	if i == nil {
		return nil
	}
	if !y.Break() {
		y.NewError(ssa.Error, "break statement not in loop or switch: raw %v", i.GetText())
	}
	return nil
}

func (y *builder) VisitReturnStatement(raw phpparser.IReturnStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ReturnStatementContext)
	if i == nil {
		return nil
	}
	if y.GetProgram().CurrentIncludingStack.Len() > 0 {
		log.Info("include stack length > 0,no emit return")
		return nil
	}
	if r := i.Expression(); r != nil {
		return y.EmitReturn([]ssa.Value{y.VisitExpression(r)})
	}
	return y.EmitReturn([]ssa.Value{y.EmitConstInstNil()})
}

func (y *builder) VisitYieldExpression(raw phpparser.IYieldExpressionContext) ssa.Value {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.YieldExpressionContext)
	if i == nil {
		return nil
	}

	if i.From() != nil {
		// yield from expression
		key := y.VisitExpression(i.Expression(0))
		_ = key
		return nil // NewIterator nil
	}

	key := y.VisitExpression(i.Expression(0))
	if i.DoubleArrow() != nil {
		// yield key => value
		val := y.VisitExpression(i.Expression(1))
		_ = key
		_ = val
		return nil
	}

	// yield key;
	return nil
}

func (y *builder) VisitGotoStatement(raw phpparser.IGotoStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.GotoStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitEmptyStatement(raw phpparser.IEmptyStatement_Context) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.EmptyStatement_Context)
	if i == nil {
		return nil
	}

	return nil
}
