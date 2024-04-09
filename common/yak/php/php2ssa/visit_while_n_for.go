package php2ssa

import (
	phpparser "github.com/yaklang/yaklang/common/yak/php/parser"
	"github.com/yaklang/yaklang/common/yak/ssa"
)

func (y *builder) VisitWhileStatement(raw phpparser.IWhileStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.WhileStatementContext)
	if i == nil {
		return nil
	}
	loopBuilder := y.CreateLoopBuilder()
	loopBuilder.SetCondition(func() ssa.Value {
		return y.VisitParentheses(i.Parentheses())
	})
	loopBuilder.SetBody(func() {
		if i.Colon() != nil {
			y.VisitInnerStatementList(i.InnerStatementList())
		} else {
			y.VisitStatement(i.Statement())
		}
	})
	loopBuilder.Finish()
	return nil
}

func (y *builder) VisitDoWhileStatement(raw phpparser.IDoWhileStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.DoWhileStatementContext)
	if i == nil {
		return nil
	}
	loopBuilder := y.CreateLoopBuilder()
	_ = loopBuilder
	loopBuilder.SetCondition(func() ssa.Value {
		return y.EmitConstInst(true)
	})
	loopBuilder.SetBody(func() {
		y.VisitStatement(i.Statement())
		y.CreateIfBuilder().SetCondition(func() ssa.Value {
			return y.VisitParentheses(i.Parentheses())
		}, func() {}).SetElse(func() {
			y.Break()
		}).Build()
	})
	loopBuilder.Finish()
	return nil
}

func (y *builder) VisitForStatement(raw phpparser.IForStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ForStatementContext)
	if i == nil {
		return nil
	}
	loopBuilder := y.CreateLoopBuilder()
	if i.ForInit() != nil {
		loopBuilder.SetFirst(func() []ssa.Value {
			return y.VisitForInit(i.ForInit())
		})
	}
	//先设置为true，在if-body前面做匹配
	loopBuilder.SetCondition(func() ssa.Value {
		return y.EmitConstInst(true)
	})
	if i.ForUpdate() != nil {
		loopBuilder.SetThird(func() []ssa.Value {
			return y.VisitForUpdate(i.ForUpdate())
		})
	}
	loopBuilder.SetBody(func() {
		if i.ExpressionList() != nil {
			list := y.VisitExpressionList(i.ExpressionList())
			for _, value := range list {
				value1 := value
				y.CreateIfBuilder().SetCondition(func() ssa.Value {
					return value1
				}, func() {
				}).SetElse(func() {
					y.Break()
				})
			}
		}
		if i.Statement() != nil {
			y.VisitStatement(i.Statement())
		} else {
			y.VisitInnerStatementList(i.InnerStatementList())
		}
	})
	loopBuilder.Finish()
	return nil
}

func (y *builder) VisitForInit(raw phpparser.IForInitContext) []ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ForInitContext)
	if i == nil {
		return nil
	}
	return y.VisitExpressionList(i.ExpressionList())
}

func (y *builder) VisitForUpdate(raw phpparser.IForUpdateContext) []ssa.Value {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ForUpdateContext)
	if i == nil {
		return nil
	}
	return y.VisitExpressionList(i.ExpressionList())
}

func (y *builder) VisitContinueStatement(raw phpparser.IContinueStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ContinueStatementContext)
	if i == nil {
		return nil
	}
	//if t := y.GetContinue(); t != nil {
	//	return y.EmitJump(t)
	//}
	return nil
}

func (y *builder) VisitForeachStatement(raw phpparser.IForeachStatementContext) interface{} {
	if y == nil || raw == nil {
		return nil
	}
	recoverRange := y.SetRange(raw)
	defer recoverRange()

	i, _ := raw.(*phpparser.ForeachStatementContext)
	if i == nil {
		return nil
	}
	return nil
}
