package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
)

func (y *builder) VisitTryCatchFinally(raw phpparser.ITryCatchFinallyContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	stmt, _ := raw.(*phpparser.TryCatchFinallyContext)
	if stmt == nil {
		return nil
	}
	//todo: try-catch-finally不支持多catch情况
	tryBuilder := y.ir.BuildTry()
	tryBuilder.BuildTryBlock(func() {
		y.VisitBlockStatement(stmt.BlockStatement())
	})
	tryBuilder.BuildFinally(func() {
		y.VisitFinallyStatement(stmt.FinallyStatement())
	})
	return nil
}
func (y *builder) VisitCatchClause(raw phpparser.ICatchClauseContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i, _ := raw.(*phpparser.CatchClauseContext)
	if i == nil {
		return nil
	}
	//todo: 这里暂时无法做
	//y.VisitQualifiedStaticTypeRef()
	return nil
}

func (y *builder) VisitFinallyStatement(raw phpparser.IFinallyStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	stmt, _ := raw.(*phpparser.FinallyStatementContext)
	if stmt == nil {
		return nil
	}
	y.VisitBlockStatement(stmt.BlockStatement())
	return nil
}

func (y *builder) VisitThrowStatement(raw phpparser.IThrowStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	stmt, _ := raw.(*phpparser.ThrowStatementContext)
	if stmt == nil {
		return nil
	}
	y.VisitExpression(stmt.Expression())
	return nil
}
