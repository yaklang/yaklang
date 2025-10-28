//go:build !no_language
// +build !no_language

package php2ssa

import (
	"github.com/google/uuid"
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitTryCatchFinally(raw phpparser.ITryCatchFinallyContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	stmt, _ := raw.(*phpparser.TryCatchFinallyContext)
	if stmt == nil {
		return nil
	}
	tryBuilder := y.BuildTry()
	tryBuilder.BuildTryBlock(func() {
		y.VisitBlockStatement(stmt.BlockStatement())
	})
	for _, catch := range stmt.AllCatchClause() {
		y.VisitCatchClause(catch, tryBuilder)
	}
	tryBuilder.BuildFinally(func() {
		y.VisitFinallyStatement(stmt.FinallyStatement())
	})
	tryBuilder.Finish()
	return nil
}
func (y *builder) VisitCatchClause(raw phpparser.ICatchClauseContext, tryBuilder *ssa.TryBuilder) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.CatchClauseContext)
	if i == nil {
		return nil
	}
	tryBuilder.BuildErrorCatch(func() string {
		if i.VarName() == nil {
			return uuid.NewString()
		} else {
			return i.VarName().GetText()
		}
	}, func() {
		y.VisitBlockStatement(i.BlockStatement())
	})
	return nil
}

func (y *builder) VisitFinallyStatement(raw phpparser.IFinallyStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	stmt, _ := raw.(*phpparser.FinallyStatementContext)
	if stmt == nil {
		return nil
	}
	y.VisitBlockStatement(stmt.BlockStatement())
	return nil
}

func (y *builder) VisitThrowStatement(raw phpparser.IThrowStatementContext) interface{} {
	if y == nil || raw == nil || y.IsStop() {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	stmt, _ := raw.(*phpparser.ThrowStatementContext)
	if stmt == nil {
		return nil
	}
	y.VisitExpression(stmt.Expression())
	return nil
}
