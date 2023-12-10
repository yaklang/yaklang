package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitWhileStatement(raw phpparser.IWhileStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.WhileStatementContext)
	if i == nil {
		return nil
	}

	loop := y.ir.BuildLoop()
	loop.BuildCondition(func() ssa.Value {
		return y.VisitParentheses(i.Parentheses())
	})
	if i.Statement() != nil {
		loop.BuildBody(func() {
			y.VisitStatement(i.Statement())
		})
	} else {
		loop.BuildBody(func() {
			y.VisitInnerStatementList(i.InnerStatementList())
		})
	}
	loop.Finish()
	return nil
}

func (y *builder) VisitDoWhileStatement(raw phpparser.IDoWhileStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.DoWhileStatementContext)
	if i == nil {
		return nil
	}

	loop := y.ir.BuildLoop()
	loop.BuildCondition(func() ssa.Value {
		return y.ir.EmitConstInst(true)
	})
	loop.BuildBody(func() {
		y.VisitStatement(i.Statement())
		y.ir.BuildIf().BuildCondition(func() ssa.Value {
			return y.VisitParentheses(i.Parentheses())
		}).BuildTrue(func() {
			y.ir.EmitJump(y.ir.GetContinue())
		}).BuildFalse(func() {
			y.ir.EmitJump(y.ir.GetBreak())
		}).Finish()
	})
	loop.Finish()
	return nil
}

func (y *builder) VisitForStatement(raw phpparser.IForStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ForStatementContext)
	if i == nil {
		return nil
	}

	return nil
}

func (y *builder) VisitContinueStatement(raw phpparser.IContinueStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ContinueStatementContext)
	if i == nil {
		return nil
	}

	if t := y.ir.GetContinue(); t != nil {
		return y.ir.EmitJump(t)
	}

	return nil
}

func (y *builder) VisitForeachStatement(raw phpparser.IForeachStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}

	i, _ := raw.(*phpparser.ForeachStatementContext)
	if i == nil {
		return nil
	}

	return nil
}
