package go2ssa

import goparser "github.com/yaklang/yaklang/common/yak/go/parser"

func (y *builder) VisitStatement(raw goparser.IStatementContext) interface{} {
	if y == nil && raw == nil {
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
