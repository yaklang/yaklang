package go2ssa

import (
	goparser "github.com/yaklang/yaklang/common/yak/go/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitStatement(raw goparser.IStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.StatementContext)
	if i == nil {
		return nil
	}
	switch {
	case i.Declaration() != nil:
		return y.VisitDeclaration(i.Declaration())
	case i.LabeledStmt() != nil:
	case i.SimpleStmt() != nil:
		return y.VisitSimpleStmt(i.SimpleStmt())
	case i.GotoStmt() != nil:
	case i.ReturnStmt() != nil:
	case i.BreakStmt() != nil:
	case i.ContinueStmt() != nil:
	case i.GoStmt() != nil:
	case i.FallthroughStmt() != nil:
	case i.Block() != nil:
		y.VisitBlock(i.Block())
	case i.IfStmt() != nil:
	case i.SwitchStmt() != nil:
	case i.SelectStmt() != nil:
	case i.ForStmt() != nil:
	case i.DeferStmt() != nil:
	}
	return nil
}
func (y *builder) VisitSimpleStmt(raw goparser.ISimpleStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.SimpleStmtContext)
	if i == nil {
		return nil
	}
	y.VisitSendStmt(i.SendStmt())
	y.VisitIncDecStmt(i.IncDecStmt())
	y.VisitAssignment(i.Assignment())
	y.VisitExpressionStmt(i.ExpressionStmt())
	y.VisitShortVarDecl(i.ShortVarDecl())
	y.VisitEmptyStmt(i.EmptyStmt())
	return nil
}
func (y *builder) VisitSendStmt(raw goparser.ISendStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.SendStmtContext)
	if i == nil {
		return nil
	}
	return nil
}
func (y *builder) VisitIncDecStmt(raw goparser.IIncDecStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.IncDecStmtContext)
	if i == nil {
		return nil
	}
	text := i.Expression().GetText()
	variable := y.ir.CreateVariable(text)
	value := y.ir.ReadValueByVariable(variable)
	switch {
	case i.PLUS_PLUS() != nil:
		after := y.ir.EmitBinOp(ssa.OpAdd, value, y.ir.EmitConstInst(1))
		y.ir.AssignVariable(variable, after)
	case i.MINUS_MINUS() != nil:
		after := y.ir.EmitBinOp(ssa.OpSub, value, y.ir.EmitConstInst(1))
		y.ir.AssignVariable(variable, after)
	}
	return nil
}
func (y *builder) VisitAssignment(raw goparser.IAssignmentContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.AssignmentContext)
	if i == nil {
		return nil
	}
	return nil
}
func (y *builder) VisitExpressionStmt(raw goparser.IExpressionStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.ExpressionStmtContext)
	if i == nil {
		return nil
	}
	y.VisitExpression(i.Expression())
	return nil
}
func (y *builder) VisitShortVarDecl(raw goparser.IShortVarDeclContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.ShortVarDeclContext)
	if i == nil {
		return nil
	}
	return nil
}
func (y *builder) VisitEmptyStmt(raw goparser.IEmptyStmtContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	i := raw.(*goparser.EmptyStmtContext)
	if i == nil {
		return nil
	}
	return nil
}
